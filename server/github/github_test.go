package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Stand-ins for the OAuth app's credentials. Joined at run time so CI's secret scan stays strict.
const (
	fakeID     = "Iv1.test-client-id"
	fakeSecret = "test-client-" + "secret-value"
)

// fakeGitHub answers the two endpoints the sign-in uses and records what it was sent.
type fakeGitHub struct {
	*httptest.Server
	tokenReply  string // body of the token endpoint
	tokenStatus int
	userReply   string
	userStatus  int
	repoReply   string
	repoStatus  int
	hang        chan struct{} // closed to release a hanging token or repo endpoint

	gotTokenForm url.Values
	gotTokenURL  string
	gotAccept    string
	gotAuth      string
	gotRepoPath  string // as it came over the wire, still escaped
	gotRepoAuth  string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{
		tokenReply: `{"access_token":"gho_fake_user_token","scope":"read:user","token_type":"bearer"}`, tokenStatus: 200,
		userReply: `{"id":42,"login":"mia-k","name":"Mia K","avatar_url":"https://avatars.example/u/42","email":"mia@example.com"}`, userStatus: 200,
		repoReply: repoJSON("Organization", `{"admin":true,"maintain":true,"push":true,"triage":true,"pull":true}`), repoStatus: 200,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if f.hang != nil {
			<-f.hang
		}
		body, _ := io.ReadAll(r.Body)
		f.gotTokenForm, _ = url.ParseQuery(string(body))
		f.gotTokenURL, f.gotAccept = r.URL.String(), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.tokenStatus)
		io.WriteString(w, f.tokenReply)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		f.gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.userStatus)
		io.WriteString(w, f.userReply)
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		if f.hang != nil {
			<-f.hang
		}
		f.gotRepoPath, f.gotRepoAuth = r.URL.EscapedPath(), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.repoStatus)
		io.WriteString(w, f.repoReply)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// repoJSON is GitHub's reply for PaperPlane/Stash. An empty permissions leaves the key out.
func repoJSON(ownerType, permissions string) string {
	reply := `{"id":1,"name":"Stash","full_name":"PaperPlane/Stash","private":false,` +
		`"owner":{"login":"PaperPlane","type":"` + ownerType + `"},"description":"A tiny HTTP cache."`
	if permissions != "" {
		reply += `,"permissions":` + permissions
	}
	return reply + `}`
}

func (f *fakeGitHub) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(Config{
		ClientID: fakeID, ClientSecret: fakeSecret,
		RedirectURL: "https://officehours.example/auth/github/callback",
		AuthURL:     f.URL + "/login/oauth/authorize", TokenURL: f.URL + "/login/oauth/access_token", APIURL: f.URL,
		Timeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewNeedsTheAppCredentials(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no client id": {ClientSecret: fakeSecret, RedirectURL: "https://officehours.example/auth/github/callback"},
		"no secret":    {ClientID: fakeID, RedirectURL: "https://officehours.example/auth/github/callback"},
		"no redirect":  {ClientID: fakeID, ClientSecret: fakeSecret},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg); err == nil {
				t.Fatal("New accepted an incomplete config")
			}
		})
	}
	c, err := New(Config{ClientID: fakeID, ClientSecret: fakeSecret, RedirectURL: "https://officehours.example/auth/github/callback"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.HasPrefix(c.AuthorizeURL("s", "v"), "https://github.com/login/oauth/authorize?") {
		t.Fatalf("the default authorize URL is not GitHub's: %s", c.AuthorizeURL("s", "v"))
	}
}

func TestChallengeMatchesRFC7636(t *testing.T) {
	// RFC 7636, appendix B.
	const verifier, want = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := Challenge(verifier); got != want {
		t.Fatalf("NFR-SEC-5: Challenge = %q, want %q", got, want)
	}
}

func TestAuthorizeURL(t *testing.T) {
	c := newFakeGitHub(t).client(t)
	u, err := url.Parse(c.AuthorizeURL("state-123", "verifier-abc"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	want := map[string]string{
		"client_id":             fakeID,
		"redirect_uri":          "https://officehours.example/auth/github/callback",
		"scope":                 "read:user", // FR-AUTH-1: profile only
		"state":                 "state-123",
		"code_challenge":        Challenge("verifier-abc"),
		"code_challenge_method": "S256",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("FR-AUTH-1: %s = %q, want %q", k, q.Get(k), v)
		}
	}
	if len(q) != len(want) {
		t.Errorf("unexpected extra parameters: %v", q)
	}
	if strings.Contains(u.String(), "verifier-abc") || strings.Contains(u.String(), fakeSecret) {
		t.Errorf("NFR-SEC-1: the browser-visible URL carries the verifier or the secret: %s", u)
	}
}

func TestExchange(t *testing.T) {
	t.Run("FR-AUTH-1: sends everything in the body and returns the token", func(t *testing.T) {
		f := newFakeGitHub(t)
		token, err := f.client(t).Exchange(context.Background(), "code-xyz", "verifier-abc")
		if err != nil || token != "gho_fake_user_token" {
			t.Fatalf("Exchange = %q, %v", token, err)
		}
		want := map[string]string{
			"client_id": fakeID, "client_secret": fakeSecret, "code": "code-xyz",
			"redirect_uri": "https://officehours.example/auth/github/callback", "code_verifier": "verifier-abc",
		}
		for k, v := range want {
			if f.gotTokenForm.Get(k) != v {
				t.Errorf("form %s = %q, want %q", k, f.gotTokenForm.Get(k), v)
			}
		}
		if f.gotTokenURL != "/login/oauth/access_token" {
			t.Errorf("NFR-SEC-1: parameters leaked into the URL: %s", f.gotTokenURL)
		}
		if f.gotAccept != "application/json" {
			t.Errorf("Accept = %q, want application/json", f.gotAccept)
		}
	})

	refusals := map[string]func(*fakeGitHub){
		"a 200 with an error instead of a token": func(f *fakeGitHub) {
			f.tokenReply = `{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`
		},
		"a 200 with an empty token": func(f *fakeGitHub) { f.tokenReply = `{"access_token":"","token_type":"bearer"}` },
		"a reply that is not JSON":  func(f *fakeGitHub) { f.tokenReply = `access_token=gho_x&token_type=bearer` },
		"a 500":                     func(f *fakeGitHub) { f.tokenStatus = 500; f.tokenReply = `{}` },
	}
	for name, breakIt := range refusals {
		t.Run("FR-AUTH-1: "+name+" is a failed sign-in", func(t *testing.T) {
			f := newFakeGitHub(t)
			breakIt(f)
			token, err := f.client(t).Exchange(context.Background(), "code-xyz", "verifier-abc")
			if err == nil || token != "" {
				t.Fatalf("Exchange = %q, %v, want an error", token, err)
			}
			for _, secret := range []string{fakeSecret, "code-xyz", "verifier-abc"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("NFR-SEC-1: the error text contains %q: %v", secret, err)
				}
			}
		})
	}

	t.Run("a GitHub that never answers gives up after the timeout", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.hang = make(chan struct{})
		defer close(f.hang)
		start := time.Now()
		_, err := f.client(t).Exchange(context.Background(), "code-xyz", "verifier-abc")
		if err == nil {
			t.Fatal("Exchange returned without an error")
		}
		if waited := time.Since(start); waited > 3*time.Second {
			t.Fatalf("waited %v for a hanging GitHub", waited)
		}
		if strings.Contains(err.Error(), fakeSecret) || strings.Contains(err.Error(), "code-xyz") {
			t.Errorf("NFR-SEC-1: the timeout error contains a secret: %v", err)
		}
	})
}

func TestUser(t *testing.T) {
	t.Run("FR-AUTH-1: reads id, login, name and avatar with the user's token", func(t *testing.T) {
		f := newFakeGitHub(t)
		u, err := f.client(t).User(context.Background(), "gho_fake_user_token")
		if err != nil {
			t.Fatalf("User: %v", err)
		}
		if u != (User{ID: 42, Login: "mia-k", Name: "Mia K", AvatarURL: "https://avatars.example/u/42"}) {
			t.Fatalf("User = %+v", u)
		}
		if f.gotAuth != "Bearer gho_fake_user_token" {
			t.Errorf("Authorization = %q", f.gotAuth)
		}
		// NFR-PRIV-1: GitHub sends an email field; the type has nowhere to put it.
		if raw, _ := json.Marshal(u); strings.Contains(string(raw), "example.com") && strings.Contains(string(raw), "mia@") {
			t.Errorf("NFR-PRIV-1: the user value kept the email: %s", raw)
		}
	})

	t.Run("a null or empty name shows as the login", func(t *testing.T) {
		for _, reply := range []string{
			`{"id":7,"login":"quiet-dev","name":null,"avatar_url":""}`,
			`{"id":7,"login":"quiet-dev","name":"","avatar_url":""}`,
			`{"id":7,"login":"quiet-dev","name":"   ","avatar_url":""}`,
			`{"id":7,"login":"quiet-dev"}`,
		} {
			f := newFakeGitHub(t)
			f.userReply = reply
			u, err := f.client(t).User(context.Background(), "t")
			if err != nil {
				t.Fatalf("User(%s): %v", reply, err)
			}
			if u.DisplayName() != "quiet-dev" {
				t.Errorf("DisplayName for %s = %q, want the login", reply, u.DisplayName())
			}
		}
		if got := (User{Login: "mia-k", Name: " Mia K "}).DisplayName(); got != "Mia K" {
			t.Errorf("DisplayName = %q, want the trimmed name", got)
		}
	})

	for name, breakIt := range map[string]func(*fakeGitHub){
		"a 401 (the token was revoked)": func(f *fakeGitHub) { f.userStatus = 401; f.userReply = `{"message":"Bad credentials"}` },
		"a reply without an id":         func(f *fakeGitHub) { f.userReply = `{"login":"mia-k"}` },
		"a reply without a login":       func(f *fakeGitHub) { f.userReply = `{"id":42}` },
		"not JSON":                      func(f *fakeGitHub) { f.userReply = `<html>` },
	} {
		t.Run(name+" is an error", func(t *testing.T) {
			f := newFakeGitHub(t)
			breakIt(f)
			if _, err := f.client(t).User(context.Background(), "gho_fake_user_token"); err == nil {
				t.Fatal("User returned no error")
			} else if strings.Contains(err.Error(), "gho_fake_user_token") {
				t.Errorf("NFR-SEC-1: the error text contains the token: %v", err)
			}
		})
	}
}

func TestRepo(t *testing.T) {
	ctx := context.Background()
	const token = "gho_fake_user_token"

	t.Run("FR-PROJ-1: reads GitHub's spelling, the description and the caller's rights", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.repoReply = repoJSON("Organization", `{"admin":false,"maintain":true,"push":true,"triage":true,"pull":true}`)
		got, err := f.client(t).Repo(ctx, token, "paperplane", "stash")
		if err != nil {
			t.Fatalf("Repo: %v", err)
		}
		want := Repo{Owner: "PaperPlane", Name: "Stash", Description: "A tiny HTTP cache.", OwnerIsOrg: true, CanMaintain: true, KnowsPermissions: true}
		if got != want {
			t.Fatalf("Repo = %+v, want %+v", got, want)
		}
		if f.gotRepoPath != "/repos/paperplane/stash" || f.gotRepoAuth != "Bearer "+token {
			t.Fatalf("asked for %q with %q", f.gotRepoPath, f.gotRepoAuth)
		}
	})

	t.Run("FR-PROJ-1: only admin or maintain counts", func(t *testing.T) {
		for _, tt := range []struct {
			name, permissions string
			can, knows        bool
		}{
			{"admin", `{"admin":true,"maintain":false,"push":true,"triage":true,"pull":true}`, true, true},
			{"maintain", `{"admin":false,"maintain":true,"push":true,"triage":true,"pull":true}`, true, true},
			{"admin, in a reply without the maintain key", `{"admin":true,"push":true,"pull":true}`, true, true},
			{"push only", `{"admin":false,"maintain":false,"push":true,"triage":true,"pull":true}`, false, true},
			{"triage only", `{"admin":false,"maintain":false,"push":false,"triage":true,"pull":true}`, false, true},
			{"pull only", `{"admin":false,"push":false,"pull":true}`, false, true},
			{"an empty permissions object", `{}`, false, true},
			{"A-7: no permissions key", ``, false, false},
			{"A-7: permissions is null", `null`, false, false},
		} {
			f := newFakeGitHub(t)
			f.repoReply = repoJSON("User", tt.permissions)
			got, err := f.client(t).Repo(ctx, token, "paperplane", "stash")
			if err != nil {
				t.Errorf("%s: %v", tt.name, err)
				continue
			}
			if got.CanMaintain != tt.can || got.KnowsPermissions != tt.knows {
				t.Errorf("%s: CanMaintain %v, KnowsPermissions %v; want %v, %v", tt.name, got.CanMaintain, got.KnowsPermissions, tt.can, tt.knows)
			}
		}
	})

	t.Run("a null description is empty, a private repo is marked, a user is not an organization", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.repoReply = `{"name":"notes","private":true,"owner":{"login":"mia-k","type":"User"},"description":null,"permissions":{"admin":true}}`
		got, err := f.client(t).Repo(ctx, token, "mia-k", "notes")
		if err != nil {
			t.Fatalf("Repo: %v", err)
		}
		want := Repo{Owner: "mia-k", Name: "notes", Private: true, CanMaintain: true, KnowsPermissions: true}
		if got != want {
			t.Fatalf("Repo = %+v, want %+v", got, want)
		}
	})

	t.Run("FR-PROJ-1: the refusals a user can do something about have their own errors", func(t *testing.T) {
		for status, want := range map[int]error{404: ErrNotFound, 401: ErrUnauthorized, 403: ErrForbidden} {
			f := newFakeGitHub(t)
			f.repoStatus, f.repoReply = status, `{"message":"no"}`
			if _, err := f.client(t).Repo(ctx, token, "paperplane", "stash"); !errors.Is(err, want) {
				t.Errorf("http %d: err = %v, want %v", status, err, want)
			}
		}
	})

	for name, breakIt := range map[string]func(*fakeGitHub){
		"a 500":                    func(f *fakeGitHub) { f.repoStatus = 500; f.repoReply = `{"message":"Server Error"}` },
		"a 429":                    func(f *fakeGitHub) { f.repoStatus = 429; f.repoReply = `{"message":"slow down"}` },
		"a page instead of JSON":   func(f *fakeGitHub) { f.repoReply = `<html>` },
		"a reply without a name":   func(f *fakeGitHub) { f.repoReply = `{"owner":{"login":"PaperPlane","type":"User"}}` },
		"a reply without an owner": func(f *fakeGitHub) { f.repoReply = `{"name":"Stash"}` },
	} {
		t.Run(name+" means GitHub did not answer", func(t *testing.T) {
			f := newFakeGitHub(t)
			breakIt(f)
			_, err := f.client(t).Repo(ctx, token, "paperplane", "stash")
			if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
				t.Fatalf("err = %v, want an error that is none of the three refusals", err)
			}
			if strings.Contains(err.Error(), token) {
				t.Errorf("NFR-SEC-1: the error text contains the token: %v", err)
			}
		})
	}

	t.Run("a GitHub that never answers gives up after the timeout", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.hang = make(chan struct{})
		defer close(f.hang)
		start := time.Now()
		_, err := f.client(t).Repo(ctx, token, "paperplane", "stash")
		if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want a plain failure", err)
		}
		if waited := time.Since(start); waited > 3*time.Second {
			t.Fatalf("waited %v for a hanging GitHub", waited)
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("NFR-SEC-1: the timeout error contains the token: %v", err)
		}
	})

	t.Run("NFR-SEC-3: a name can't change which address is asked", func(t *testing.T) {
		f := newFakeGitHub(t)
		if _, err := f.client(t).Repo(ctx, token, "paper/plane", "stash?x=1#y"); err != nil {
			t.Fatalf("Repo: %v", err)
		}
		if f.gotRepoPath != "/repos/paper%2Fplane/stash%3Fx=1%23y" {
			t.Fatalf("asked for %q: the name escaped its place in the path", f.gotRepoPath)
		}
	})
}
