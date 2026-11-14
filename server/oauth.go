package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"officehours/pages"
	"officehours/session"
	"officehours/store"
)

// The OAuth 2.1 sign-in an agent's app goes through before it may use /mcp (FR-AGENT-2), as the
// MCP spec (2026-07-28, "Authorization") asks: protected-resource and authorization-server
// metadata, dynamic registration, a consent page, and a token endpoint for codes with PKCE S256.
//
// Nothing is stored but one row per approved app (agent_grants). Every other value is sealed
// with AUTH_SECRET by the session codec, under its own name, so one kind never passes as another:
// the client_id an app registers holds its name and redirect URIs; the consent form holds the
// checked request and whose page it was; the code and the access token hold the person, their
// GitHub token included (the owner's decision of 2026-11-14), the grant and the resource. There
// are no refresh tokens: an access token lasts as long as a web sign-in, and /mcp checks its
// grant on every request, so Disconnect ends it at once.

const (
	mcpPath              = "/mcp"
	resourceMetadataPath = "/.well-known/oauth-protected-resource"
	serverMetadataPath   = "/.well-known/oauth-authorization-server"
	registerPath         = "/oauth/register"
	authorizePath        = "/oauth/authorize"
	tokenPath            = "/oauth/token"
	agentScope           = "officehours" // one scope: an agent can do what its person can

	clientSeal  = "oh_client"
	consentSeal = "oh_consent"
	codeSeal    = "oh_code"
	accessSeal  = "oh_access"

	clientTTL  = 5 * 365 * 24 * time.Hour // a new AUTH_SECRET ends every registration sooner
	consentTTL = 10 * time.Minute
	codeTTL    = 5 * time.Minute

	// The whole authorization request rides through GitHub's sign-in in the oh_oauth cookie,
	// which a browser drops past 4 KB: these caps keep the largest one under it.
	maxRedirects   = 3
	maxRedirectLen = 256
	maxAppName     = 60
	maxState       = 512
	maxOAuthBody   = 16 << 10
)

// grantStore is the part of the store the agent sign-in needs.
type grantStore interface {
	CreateGrant(ctx context.Context, g store.Grant) error
	UseGrant(ctx context.Context, id string) (bool, error)
	RevokeGrantByID(ctx context.Context, id string) error
	RevokeGrant(ctx context.Context, userID, id string) (bool, error)
	ListGrants(ctx context.Context, userID string) ([]store.Grant, error)
}

// clientInfo is what a registered app's client_id holds: registering keeps nothing here.
type clientInfo struct {
	Name      string   `json:"n"`
	Redirects []string `json:"r"`
}

// consentForm is what the consent page's buttons send back: the checked request, and whose page
// it was.
type consentForm struct {
	UserID      string `json:"u"`
	Client      string `json:"c"` // clientHash of the client_id
	App         string `json:"a"`
	RedirectURI string `json:"r"`
	State       string `json:"s"`
	Challenge   string `json:"x"`
	Resource    string `json:"o"`
}

// authCode is an authorization code: the grant to make, for whom, and what the token request must
// show again.
type authCode struct {
	GrantID     string       `json:"g"`
	User        session.User `json:"u"`
	Client      string       `json:"c"`
	App         string       `json:"a"`
	RedirectURI string       `json:"r"`
	Challenge   string       `json:"x"`
	Resource    string       `json:"o"`
}

// agentToken is an access token: its grant, its person, and the resource it is for.
type agentToken struct {
	GrantID  string       `json:"g"`
	User     session.User `json:"u"`
	Audience string       `json:"aud"`
}

var (
	challengeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)       // S256 of 32 bytes, base64url
	verifierRe  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`) // RFC 7636 §4.1
)

// deniedSchemes can't be an app's own callback scheme: they run code, read files or reach the
// network some other way.
var deniedSchemes = map[string]bool{
	"javascript": true, "data": true, "file": true, "blob": true, "about": true, "vbscript": true,
	"filesystem": true, "view-source": true, "ws": true, "wss": true, "ftp": true, "mailto": true, "tel": true, "sms": true,
}

// agentResource is the MCP endpoint's address: what every token is for.
func (d deps) agentResource() string { return d.guard.Origin() + mcpPath }

// clientHash names a client_id in codes and forms without carrying it whole.
func clientHash(clientID string) string {
	sum := sha256.Sum256([]byte(clientID))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// isLoopback reports whether host is this computer.
func isLoopback(host string) bool { return host == "localhost" || host == "127.0.0.1" || host == "::1" }

// redirectAllowed reports whether an app may register uri as a callback: https anywhere, http only
// on this computer, or an app's own scheme; never one that could hand a code to someone else.
func redirectAllowed(uri string) bool {
	if uri == "" || len(uri) > maxRedirectLen || strings.Contains(uri, "#") {
		return false
	}
	u, err := url.Parse(uri)
	if err != nil || u.User != nil || u.Scheme == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return isLoopback(u.Hostname())
	}
	return !deniedSchemes[u.Scheme]
}

// redirectMatches compares a callback with a registered one: equal, or both http on the same
// loopback host, path and query, in any port. An app on this computer takes a free port each time
// it runs (RFC 8252 §7.3).
func redirectMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	a, errA := url.Parse(registered)
	b, errB := url.Parse(requested)
	return errA == nil && errB == nil && a.Scheme == "http" && b.Scheme == "http" && isLoopback(a.Hostname()) &&
		a.Hostname() == b.Hostname() && a.EscapedPath() == b.EscapedPath() && a.RawQuery == b.RawQuery
}

// returnsTo is where a callback sends the person, as the consent page and the dashboard name it.
func returnsTo(uri string) string {
	u, err := url.Parse(uri)
	switch {
	case err != nil:
		return uri
	case u.Scheme == "http" || u.Scheme == "https":
		return u.Hostname()
	case u.Host != "":
		return u.Scheme + "://" + u.Host
	}
	return u.Scheme + ":"
}

// sameResource compares a resource indicator with ours, ignoring the case of the scheme and host
// and a trailing slash.
func sameResource(got, ours string) bool {
	g, errG := url.Parse(got)
	o, errO := url.Parse(ours)
	return errG == nil && errO == nil && strings.EqualFold(g.Scheme, o.Scheme) && strings.EqualFold(g.Host, o.Host) &&
		strings.TrimSuffix(g.EscapedPath(), "/") == o.EscapedPath() && g.RawQuery == "" && g.Fragment == ""
}

// pkceMatches checks a code verifier against the challenge it was sent with (S256).
func pkceMatches(verifier, challenge string) bool {
	if !verifierRe.MatchString(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}

// allowAnyOrigin lets a page on another origin read the answer. These endpoints read no cookie,
// and the MCP Inspector's page calls them.
func allowAnyOrigin(w http.ResponseWriter) { w.Header().Set("Access-Control-Allow-Origin", "*") }

// corsPreflight tells a browser that a page on another origin may make such a call.
func corsPreflight(w http.ResponseWriter, _ *http.Request) {
	allowAnyOrigin(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, MCP-Protocol-Version")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// writeOAuthJSON answers in OAuth's own shapes, not the API's envelope.
func writeOAuthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("oauth: write: %v", err)
	}
}

// oauthError is an error in RFC 6749's shape.
func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeOAuthJSON(w, status, map[string]string{"error": code, "error_description": description})
}

// oauthResourceMetadata is RFC 9728's document for /mcp, at the root and at /mcp's own path.
func (d deps) oauthResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	allowAnyOrigin(w)
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource": d.agentResource(), "resource_name": "Office Hours",
		"authorization_servers": []string{d.guard.Origin()}, "scopes_supported": []string{agentScope},
		"bearer_methods_supported": []string{"header"}, "resource_documentation": d.guard.Origin() + "/llms.txt",
	})
}

// oauthServerMetadata is RFC 8414's document. The issuer is our origin exactly, as clients
// compare it byte for byte.
func (d deps) oauthServerMetadata(w http.ResponseWriter, _ *http.Request) {
	allowAnyOrigin(w)
	w.Header().Set("Cache-Control", "public, max-age=300")
	origin := d.guard.Origin()
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                 origin,
		"authorization_endpoint": origin + authorizePath, "token_endpoint": origin + tokenPath, "registration_endpoint": origin + registerPath,
		"scopes_supported": []string{agentScope}, "response_types_supported": []string{"code"},
		"grant_types_supported": []string{"authorization_code"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true,
		"service_documentation": origin + "/llms.txt",
	})
}

// oauthRegister is RFC 7591's registration, kept nowhere: the client_id is the app's name and
// callbacks, sealed. Fields it doesn't use are ignored; the answer says what it got instead (a
// public client, codes only).
func (d deps) oauthRegister(w http.ResponseWriter, r *http.Request) {
	allowAnyOrigin(w)
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Name      string   `json:"client_name"`
		Redirects []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOAuthBody)).Decode(&in); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "Send the registration as JSON.")
		return
	}
	if len(in.Redirects) == 0 || len(in.Redirects) > maxRedirects || !slicesAll(in.Redirects, redirectAllowed) {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri",
			"Give one to three redirect URIs of at most 256 characters, without a fragment: https, http on localhost, or an app's own scheme.")
		return
	}
	name := clip(cleanLine(in.Name), maxAppName)
	if name == "" {
		name = "An unnamed app"
	}
	clientID, err := d.sessions.Seal(clientSeal, clientInfo{Name: name, Redirects: in.Redirects}, clientTTL)
	if err != nil {
		log.Printf("oauth register: seal: %v", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "Something went wrong on our side. Try again.")
		return
	}
	writeOAuthJSON(w, http.StatusCreated, map[string]any{
		"client_id": clientID, "client_id_issued_at": now().Unix(), "client_name": name, "redirect_uris": in.Redirects,
		"grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

func slicesAll(list []string, ok func(string) bool) bool {
	return !slices.ContainsFunc(list, func(s string) bool { return !ok(s) })
}

// consentHeaders keep the consent page out of frames, caches and Referer headers.
func consentHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// oauthAuthorize is GET /oauth/authorize: the consent page, after our own GitHub sign-in when
// needed. It never sends the browser to the app: every problem is a page here, since an app that
// registered any https callback could otherwise use our origin to redirect anywhere (OAuth 2.1
// §7.12.2). Only the person's Allow or Cancel goes back (oauthDecide).
func (d deps) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	consentHeaders(w)
	if d.grants == nil {
		writePage(w, http.StatusServiceUnavailable, pages.Notice{Title: "Apps can't connect to this server yet",
			Text: "This server has no database to keep connections in. Try again later."}, "no-store")
		return
	}
	req, problem := d.authorizeRequest(r.URL.Query())
	if problem != "" {
		writePage(w, http.StatusBadRequest, pages.Notice{Title: "This app's request can't be used", Text: problem,
			Link: "/", LinkText: "Back to the start"}, "no-store")
		return
	}
	u, err := d.guard.User(r)
	if err != nil {
		http.Redirect(w, r, "/auth/github/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	req.UserID = u.ID
	form, err := d.sessions.Seal(consentSeal, req, consentTTL)
	if err != nil {
		log.Printf("oauth authorize: seal: %v", err)
		writePage(w, http.StatusInternalServerError, pages.Notice{Title: "Something went wrong on our side",
			Text: "Try connecting the app again in a moment."}, "no-store")
		return
	}
	host := returnsTo(req.RedirectURI)
	writePage(w, http.StatusOK, pages.Consent{App: req.App, ReturnsTo: host, OnYourComputer: isLoopback(host),
		Login: u.Login, AvatarURL: u.AvatarURL, Form: form}, "no-store")
}

// authorizeRequest checks an authorization request. problem is a sentence for the person when it
// can't be used; nothing of it goes to the app.
func (d deps) authorizeRequest(q url.Values) (consentForm, string) {
	var info clientInfo
	clientID := q.Get("client_id")
	if clientID == "" || d.sessions.Open(clientSeal, clientID, &info) != nil {
		return consentForm{}, "Office Hours doesn't know this app. Start connecting it again from the app."
	}
	redirect := q.Get("redirect_uri")
	if !redirectAllowed(redirect) || !slices.ContainsFunc(info.Redirects, func(reg string) bool { return redirectMatches(reg, redirect) }) {
		return consentForm{}, "The app asked to send you back to an address it didn't register."
	}
	if q.Get("response_type") != "code" {
		return consentForm{}, "The app asked for a kind of sign-in Office Hours doesn't offer."
	}
	challenge := q.Get("code_challenge")
	if q.Get("code_challenge_method") != "S256" || !challengeRe.MatchString(challenge) {
		return consentForm{}, "The app's request isn't protected the way Office Hours needs (PKCE with S256)."
	}
	if resource := q.Get("resource"); resource != "" && !sameResource(resource, d.agentResource()) {
		return consentForm{}, "The app asked for access to something other than Office Hours."
	}
	state := q.Get("state")
	if len(state) > maxState {
		return consentForm{}, "The app's request is too long."
	}
	return consentForm{Client: clientHash(clientID), App: info.Name, RedirectURI: redirect, State: state,
		Challenge: challenge, Resource: d.agentResource()}, ""
}

// oauthDecide is POST /oauth/authorize: the person's Allow or Cancel on the consent page. Only a
// form our page made for this same person counts, sent from our origin.
func (d deps) oauthDecide(w http.ResponseWriter, r *http.Request) {
	consentHeaders(w)
	refuse := func(status int, text string) {
		writePage(w, status, pages.Notice{Title: "Nothing was connected", Text: text, Link: "/dashboard", LinkText: "Go to your dashboard"}, "no-store")
	}
	if err := d.guard.SameOrigin(r); err != nil {
		refuse(http.StatusForbidden, "This choice didn't come from Office Hours' own page.")
		return
	}
	u, err := d.guard.User(r)
	if err != nil {
		refuse(http.StatusUnauthorized, "Sign in again, then start connecting the app again from the app.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthBody)
	var c consentForm
	if r.ParseForm() != nil || d.sessions.Open(consentSeal, r.PostForm.Get("consent"), &c) != nil || c.UserID != u.ID {
		refuse(http.StatusBadRequest, "This page expired, or was made for someone else. Start connecting the app again from the app.")
		return
	}
	back := url.Values{"iss": {d.guard.Origin()}}
	if c.State != "" {
		back.Set("state", c.State)
	}
	switch r.PostForm.Get("decision") {
	case "allow":
		code, err := d.sessions.Seal(codeSeal, authCode{GrantID: rand.Text(), User: u, Client: c.Client, App: c.App,
			RedirectURI: c.RedirectURI, Challenge: c.Challenge, Resource: c.Resource}, codeTTL)
		if err != nil {
			log.Printf("oauth decide: seal: %v", err)
			refuse(http.StatusInternalServerError, "Something went wrong on our side. Try again in a moment.")
			return
		}
		back.Set("code", code)
	case "deny":
		back.Set("error", "access_denied")
	default:
		refuse(http.StatusBadRequest, "Choose Allow or Cancel.")
		return
	}
	target, err := url.Parse(c.RedirectURI)
	if err != nil { // checked at registration and again on the consent page
		refuse(http.StatusBadRequest, "The app's address can't be used.")
		return
	}
	q := target.Query()
	for k, v := range back {
		q[k] = v
	}
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// oauthToken is POST /oauth/token: a code, its verifier and the same app and callback buy an
// access token for /mcp, and the grant is made. Everything is checked before the grant is
// written, so a code thief without the verifier can't use the code up; a code that comes back
// after its grant was made ends that grant (RFC 6749 §4.1.2).
func (d deps) oauthToken(w http.ResponseWriter, r *http.Request) {
	allowAnyOrigin(w)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if d.grants == nil {
		oauthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "This server has no database to keep connections in.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthBody)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "Send the token request as a form.")
		return
	}
	form := r.PostForm
	switch form.Get("grant_type") {
	case "authorization_code":
	case "":
		oauthError(w, http.StatusBadRequest, "invalid_request", "grant_type is missing.")
		return
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only authorization_code is offered: when a token runs out, sign in again.")
		return
	}
	raw := form.Get("code")
	if raw == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code is missing.")
		return
	}
	var c authCode
	if d.sessions.Open(codeSeal, raw, &c) != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code is unknown or has expired.")
		return
	}
	clientID := form.Get("client_id")
	if user, _, ok := r.BasicAuth(); ok && clientID == "" {
		clientID, _ = url.QueryUnescape(user)
	}
	switch {
	case clientID == "" || clientHash(clientID) != c.Client:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code was issued to another app.")
		return
	case form.Get("redirect_uri") != c.RedirectURI:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri is not the one the code was issued for.")
		return
	case !pkceMatches(form.Get("code_verifier"), c.Challenge):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The code verifier doesn't match.")
		return
	}
	if resource := form.Get("resource"); resource != "" && !sameResource(resource, c.Resource) {
		oauthError(w, http.StatusBadRequest, "invalid_target", "The code is for "+c.Resource+".")
		return
	}
	err := d.grants.CreateGrant(r.Context(), store.Grant{ID: c.GrantID, UserID: c.User.ID, ClientName: c.App, RedirectHost: returnsTo(c.RedirectURI)})
	switch {
	case errors.Is(err, store.ErrGrantExists):
		log.Printf("oauth token: a code came back after its grant was made; ending grant %s", c.GrantID)
		if err := d.grants.RevokeGrantByID(r.Context(), c.GrantID); err != nil {
			log.Printf("oauth token: end grant %s: %v", c.GrantID, err)
		}
		oauthError(w, http.StatusBadRequest, "invalid_grant", "This code was used already.")
		return
	case errors.Is(err, store.ErrUnknownUser):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "The person who approved this app is no longer signed up.")
		return
	case err != nil:
		log.Printf("oauth token: create grant: %v", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "Something went wrong on our side. Try again.")
		return
	}
	access, err := d.sessions.Seal(accessSeal, agentToken{GrantID: c.GrantID, User: c.User, Audience: c.Resource}, session.Lifetime)
	if err != nil {
		log.Printf("oauth token: seal: %v", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "Something went wrong on our side. Try again.")
		return
	}
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int64(session.Lifetime / time.Second), "scope": agentScope,
	})
}
