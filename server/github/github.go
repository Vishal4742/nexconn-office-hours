// Package github is the only place that talks to GitHub: the OAuth sign-in (state and PKCE are the
// caller's job, the requests are ours) and read-only API calls made with the user's own token.
// The client secret and user tokens never appear in an error or a URL (NFR-SEC-1).
package github

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config is the OAuth app and where GitHub lives. The three URLs are only set in tests.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string        // APP_URL + /auth/github/callback, exactly as registered with GitHub
	AuthURL      string        // default https://github.com/login/oauth/authorize
	TokenURL     string        // default https://github.com/login/oauth/access_token
	APIURL       string        // default https://api.github.com
	Timeout      time.Duration // per request; 5 s when zero
}

// Client talks to GitHub for one OAuth app.
type Client struct {
	cfg  Config
	http *http.Client
}

// maxReply caps what we read from GitHub: the replies we use are a few hundred bytes.
const maxReply = 1 << 20

// New builds a client. It has its own timeout: http.DefaultClient would wait forever.
func New(cfg Config) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("github: client id, client secret and redirect URL are all required")
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.github.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}, nil
}

// Challenge is the PKCE code challenge for a verifier (RFC 7636, method S256).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is where the browser goes to sign in. Only the profile is asked for (FR-AUTH-1).
func (c *Client) AuthorizeURL(state, verifier string) string {
	q := url.Values{
		"client_id":             {c.cfg.ClientID},
		"redirect_uri":          {c.cfg.RedirectURL},
		"scope":                 {"read:user"},
		"state":                 {state},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	return c.cfg.AuthURL + "?" + q.Encode()
}

// Exchange trades the code from the callback for the user's access token. Everything goes in the
// request body: Go quotes URLs in its errors, and bodies are never logged.
func (c *Client) Exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURL},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("github: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var reply struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"` // GitHub reports a bad code with HTTP 200 and this field
	}
	status, err := c.do(req, &reply)
	if err != nil {
		return "", fmt.Errorf("github: token exchange: %w", err)
	}
	if status != http.StatusOK || reply.AccessToken == "" {
		// reply.Error is one of GitHub's short codes (bad_verification_code, …), safe to log.
		return "", fmt.Errorf("github: token exchange refused: http %d, error %q", status, reply.Error)
	}
	return reply.AccessToken, nil
}

// User is the public profile of the person who signed in. GitHub also sends an email field; it
// has no place here (NFR-PRIV-1).
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"` // null for many accounts: see DisplayName
	AvatarURL string `json:"avatar_url"`
}

// DisplayName is what the app calls the user everywhere: their name, or their login when GitHub
// has no name for them. Chat UI and Nexconn both need a non-empty name.
func (u User) DisplayName() string {
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	return u.Login
}

// User reads the profile that belongs to an access token.
func (c *Client) User(ctx context.Context, token string) (User, error) {
	req, err := c.apiRequest(ctx, token, "/user")
	if err != nil {
		return User{}, err
	}
	var u User
	status, err := c.do(req, &u)
	if err != nil {
		return User{}, fmt.Errorf("github: read user: %w", err)
	}
	if status != http.StatusOK || u.ID == 0 || u.Login == "" {
		return User{}, fmt.Errorf("github: read user: http %d, no usable profile in the reply", status)
	}
	return u, nil
}

// The refusals of Repo that the person asking can do something about. Any other error means
// GitHub did not give a usable answer.
var (
	ErrNotFound     = errors.New("github: no such repository, or not one this token can see")
	ErrUnauthorized = errors.New("github: the token was refused")
	ErrForbidden    = errors.New("github: access to the repository was refused")
)

// Repo is a repository as the signed-in user sees it.
type Repo struct {
	ID          int64  // stays the same when the repository is renamed or moved; a name can be taken by another
	Owner       string // GitHub's own spelling, which may differ in case from what was asked
	Name        string
	Description string
	Private     bool
	OwnerIsOrg  bool
	// CanMaintain is permissions.admin or permissions.maintain (FR-PROJ-1). KnowsPermissions is
	// false when the reply had no permissions at all: then CanMaintain says nothing.
	CanMaintain      bool
	KnowsPermissions bool
}

// Repo asks GitHub about one repository with the user's own token. A token with the read:user
// scope sees public repositories only; a private one answers like one that does not exist.
func (c *Client) Repo(ctx context.Context, token, owner, name string) (Repo, error) {
	// Escaped, so a name can never reach another endpoint with the user's token attached.
	req, err := c.apiRequest(ctx, token, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name))
	if err != nil {
		return Repo{}, err
	}
	var reply struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"` // null for many repositories
		Private     bool   `json:"private"`
		Owner       struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
		Permissions *struct {
			Admin    bool `json:"admin"`
			Maintain bool `json:"maintain"`
		} `json:"permissions"`
	}
	status, err := c.do(req, &reply)
	if err != nil {
		return Repo{}, fmt.Errorf("github: read repository: %w", err)
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return Repo{}, ErrNotFound
	case http.StatusUnauthorized:
		return Repo{}, ErrUnauthorized
	case http.StatusForbidden:
		return Repo{}, ErrForbidden
	default:
		return Repo{}, fmt.Errorf("github: read repository: http %d", status)
	}
	if reply.ID == 0 || reply.Name == "" || reply.Owner.Login == "" {
		return Repo{}, errors.New("github: read repository: no usable repository in the reply")
	}
	repo := Repo{
		ID: reply.ID, Owner: reply.Owner.Login, Name: reply.Name, Description: reply.Description,
		Private: reply.Private, OwnerIsOrg: reply.Owner.Type == "Organization",
	}
	if reply.Permissions != nil {
		repo.KnowsPermissions = true
		repo.CanMaintain = reply.Permissions.Admin || reply.Permissions.Maintain
	}
	return repo, nil
}

func (c *Client) apiRequest(ctx context.Context, token, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("github: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// do sends the request and decodes a JSON reply into out. A reply that is not JSON leaves out
// untouched and is not an error by itself: the caller judges by status and content.
func (c *Client) do(req *http.Request, out any) (int, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return resp.StatusCode, err
	}
	_ = json.Unmarshal(body, out) // see the comment above
	return resp.StatusCode, nil
}
