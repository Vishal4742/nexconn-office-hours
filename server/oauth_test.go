package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"officehours/session"
	"officehours/store"
)

// The OAuth sign-in an agent's app goes through (FR-AGENT-2) and a person's connected agents
// (FR-AGENT-3).

const (
	agentRedirect = "http://127.0.0.1:33418/callback"
	agentResource = testSite + "/mcp"
)

// fakeGrants keeps grants in memory, like the agent_grants table.
type fakeGrants struct {
	rows    map[string]store.Grant
	order   []string
	revoked map[string]bool
	err     error
}

func (f *fakeGrants) CreateGrant(_ context.Context, g store.Grant) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.rows[g.ID]; ok {
		return store.ErrGrantExists
	}
	g.CreatedAt = time.Now()
	f.rows[g.ID] = g
	f.order = append(f.order, g.ID)
	return nil
}

func (f *fakeGrants) UseGrant(_ context.Context, id string) (bool, error) {
	_, ok := f.rows[id]
	return ok && !f.revoked[id], f.err
}

func (f *fakeGrants) RevokeGrantByID(_ context.Context, id string) error {
	f.revoked[id] = true
	return f.err
}

func (f *fakeGrants) RevokeGrant(_ context.Context, userID, id string) (bool, error) {
	g, ok := f.rows[id]
	if f.err != nil || !ok || g.UserID != userID || f.revoked[id] {
		return false, f.err
	}
	f.revoked[id] = true
	return true, nil
}

func (f *fakeGrants) ListGrants(_ context.Context, userID string) ([]store.Grant, error) {
	var out []store.Grant
	for _, id := range slices.Backward(f.order) {
		if g := f.rows[id]; g.UserID == userID && !f.revoked[id] {
			out = append(out, g)
		}
	}
	return out, f.err
}

type agentApp struct {
	*queueApp
	grants *fakeGrants
}

func newAgentApp(t *testing.T) *agentApp {
	t.Helper()
	a := &agentApp{queueApp: newQueueApp(t), grants: &fakeGrants{rows: map[string]store.Grant{}, revoked: map[string]bool{}}}
	a.deps.grants = a.grants
	a.remux()
	return a
}

// send makes one request the way a browser or an agent's app would; mutate sets the rest.
func send(mux http.Handler, method, target, contentType, body string, cookie *http.Cookie, mutate ...func(*http.Request)) (*http.Response, string) {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for _, m := range mutate {
		m(r)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	text, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(text)
}

func fromOurSite(r *http.Request) { r.Header.Set("Origin", testSite) }

// pkce is a fresh verifier and its S256 challenge.
func pkce() (verifier, challenge string) {
	verifier = rand.Text() + rand.Text()
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizeQuery(clientID, redirect, challenge, state string) url.Values {
	return url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {state},
		"resource": {agentResource}, "scope": {"officehours"},
	}
}

func jsonOf(t *testing.T, body string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("not JSON (%v): %s", err, body)
	}
	return v
}

// register registers an app and returns its client_id.
func (a *agentApp) register(t *testing.T, name string, redirects ...string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"client_name": name, "redirect_uris": redirects})
	res, text := send(a.mux, "POST", "/oauth/register", "application/json", string(body), nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d: %s", res.StatusCode, text)
	}
	return jsonOf(t, text)["client_id"].(string)
}

var consentRe = regexp.MustCompile(`name="consent" value="([^"]+)"`)

// consent opens the consent page as the cookie's person and returns its sealed form.
func (a *agentApp) consent(t *testing.T, cookie *http.Cookie, q url.Values) string {
	t.Helper()
	res, page := send(a.mux, "GET", "/oauth/authorize?"+q.Encode(), "", "", cookie)
	m := consentRe.FindStringSubmatch(page)
	if res.StatusCode != http.StatusOK || m == nil {
		t.Fatalf("consent page: status %d:\n%s", res.StatusCode, page)
	}
	return html.UnescapeString(m[1])
}

func (a *agentApp) decide(cookie *http.Cookie, form, decision string, mutate ...func(*http.Request)) *http.Response {
	res, _ := send(a.mux, "POST", "/oauth/authorize", "application/x-www-form-urlencoded",
		url.Values{"consent": {form}, "decision": {decision}}.Encode(), cookie, append([]func(*http.Request){fromOurSite}, mutate...)...)
	return res
}

func (a *agentApp) token(form url.Values, mutate ...func(*http.Request)) (*http.Response, string) {
	return send(a.mux, "POST", "/oauth/token", "application/x-www-form-urlencoded", form.Encode(), nil, mutate...)
}

// approved runs registration and consent for user, and returns what the token request needs.
func (a *agentApp) approved(t *testing.T, user string) (clientID, verifier, code string) {
	t.Helper()
	clientID = a.register(t, "Claude Code", agentRedirect)
	verifier, challenge := pkce()
	form := a.consent(t, a.as(t, user), authorizeQuery(clientID, agentRedirect, challenge, "st-1"))
	res := a.decide(a.as(t, user), form, "allow")
	loc, err := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusFound || err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("allow: status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
	}
	return clientID, verifier, loc.Query().Get("code")
}

func exchange(clientID, verifier, code string) url.Values {
	return url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {agentRedirect},
		"client_id": {clientID}, "code_verifier": {verifier}, "resource": {agentResource},
	}
}

// connect runs the whole sign-in for user and returns the agent's access token.
func (a *agentApp) connect(t *testing.T, user string) string {
	t.Helper()
	clientID, verifier, code := a.approved(t, user)
	res, text := a.token(exchange(clientID, verifier, code))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("token: status %d: %s", res.StatusCode, text)
	}
	return jsonOf(t, text)["access_token"].(string)
}

func TestAgentMetadata(t *testing.T) {
	a := newAgentApp(t)
	spoofed := func(r *http.Request) { r.Host = "evil.example"; r.Header.Set("X-Forwarded-Host", "evil.example") }

	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		res, text := send(a.mux, "GET", path, "", "", nil, spoofed)
		doc := jsonOf(t, text)
		if res.StatusCode != http.StatusOK || doc["resource"] != agentResource ||
			strings.Join(anyStrings(doc["authorization_servers"]), " ") != testSite ||
			strings.Join(anyStrings(doc["scopes_supported"]), " ") != "officehours" ||
			strings.Join(anyStrings(doc["bearer_methods_supported"]), " ") != "header" {
			t.Errorf("RFC 9728 %s: status %d, %v", path, res.StatusCode, doc)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: no CORS for the Inspector's page", path)
		}
	}

	res, text := send(a.mux, "GET", "/.well-known/oauth-authorization-server", "", "", nil, spoofed)
	doc := jsonOf(t, text)
	for key, want := range map[string]string{
		"issuer": testSite, "authorization_endpoint": testSite + "/oauth/authorize",
		"token_endpoint": testSite + "/oauth/token", "registration_endpoint": testSite + "/oauth/register",
	} {
		if doc[key] != want {
			t.Errorf("RFC 8414 %s = %v, want %s", key, doc[key], want)
		}
	}
	for key, want := range map[string]string{
		"response_types_supported": "code", "grant_types_supported": "authorization_code",
		"code_challenge_methods_supported": "S256", "token_endpoint_auth_methods_supported": "none",
	} {
		if got := strings.Join(anyStrings(doc[key]), " "); got != want {
			t.Errorf("RFC 8414 %s = %q, want %q", key, got, want)
		}
	}
	if doc["authorization_response_iss_parameter_supported"] != true || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("RFC 9207 flag or CORS missing: %v", doc)
	}

	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server", "/oauth/register", "/oauth/token"} {
		res, _ := send(a.mux, "OPTIONS", path, "", "", nil, func(r *http.Request) {
			r.Header.Set("Origin", "http://localhost:6274")
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "content-type,mcp-protocol-version")
		})
		allowed := strings.ToLower(res.Header.Get("Access-Control-Allow-Headers"))
		if res.StatusCode != http.StatusNoContent || res.Header.Get("Access-Control-Allow-Origin") != "*" ||
			!strings.Contains(allowed, "content-type") || !strings.Contains(allowed, "mcp-protocol-version") {
			t.Errorf("preflight %s: status %d, headers %v", path, res.StatusCode, res.Header)
		}
	}
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

func TestAgentRegister(t *testing.T) {
	a := newAgentApp(t)

	t.Run("RFC 7591: what the app asked for, as a public client of ours, and nothing it can't use", func(t *testing.T) {
		res, text := send(a.mux, "POST", "/oauth/register", "application/json",
			`{"client_name":"Claude Code","redirect_uris":["http://localhost:51234/callback"],"grant_types":["authorization_code","refresh_token"],"token_endpoint_auth_method":"client_secret_basic","software_id":"x"}`, nil)
		doc := jsonOf(t, text)
		if res.StatusCode != http.StatusCreated || doc["client_id"] == "" || doc["client_id_issued_at"] == nil || doc["client_name"] != "Claude Code" ||
			strings.Join(anyStrings(doc["redirect_uris"]), " ") != "http://localhost:51234/callback" ||
			strings.Join(anyStrings(doc["grant_types"]), " ") != "authorization_code" ||
			strings.Join(anyStrings(doc["response_types"]), " ") != "code" || doc["token_endpoint_auth_method"] != "none" {
			t.Fatalf("status %d: %v", res.StatusCode, doc)
		}
		if _, has := doc["client_secret"]; has {
			t.Error("a public client got a secret")
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("headers %v", res.Header)
		}
	})

	t.Run("the callbacks real apps use are accepted", func(t *testing.T) {
		for _, uri := range []string{
			"https://claude.ai/api/mcp/auth_callback", "https://chatgpt.com/connector_platform_oauth_redirect",
			"http://127.0.0.1:33418", "http://[::1]:8080/cb", "http://localhost/callback",
			"cursor://anysphere.cursor-mcp/oauth/callback", "https://vscode.dev/redirect",
		} {
			body, _ := json.Marshal(map[string]any{"client_name": "x", "redirect_uris": []string{uri}})
			if res, text := send(a.mux, "POST", "/oauth/register", "application/json", string(body), nil); res.StatusCode != http.StatusCreated {
				t.Errorf("%s: status %d: %s", uri, res.StatusCode, text)
			}
		}
	})

	t.Run("a callback that could leak a code, or a request too big for the sign-in cookie, is refused", func(t *testing.T) {
		for name, uris := range map[string][]string{
			"plain http elsewhere":   {"http://evil.example/cb"},
			"javascript":             {"javascript:alert(1)"},
			"data":                   {"data:text/html,hi"},
			"file":                   {"file:///etc/passwd"},
			"a fragment":             {"https://claude.ai/cb#x"},
			"a login in the address": {"https://claude.ai@evil.example/cb"},
			"empty":                  {""},
			"no scheme":              {"claude.ai/cb"},
			"too many":               {"https://a.example/1", "https://a.example/2", "https://a.example/3", "https://a.example/4"},
			"too long":               {"https://a.example/" + strings.Repeat("x", 250)},
			"none":                   {},
		} {
			body, _ := json.Marshal(map[string]any{"client_name": "x", "redirect_uris": uris})
			res, text := send(a.mux, "POST", "/oauth/register", "application/json", string(body), nil)
			if res.StatusCode != http.StatusBadRequest || jsonOf(t, text)["error"] != "invalid_redirect_uri" {
				t.Errorf("%s: status %d: %s", name, res.StatusCode, text)
			}
		}
		res, text := send(a.mux, "POST", "/oauth/register", "application/json", `{"redirect_uris":`, nil)
		if res.StatusCode != http.StatusBadRequest || jsonOf(t, text)["error"] != "invalid_client_metadata" {
			t.Errorf("broken JSON: status %d: %s", res.StatusCode, text)
		}
	})

	t.Run("the app's name is one line of at most 60 characters, and something when missing", func(t *testing.T) {
		for name, want := range map[string]string{"  Claude\n  Code ": "Claude Code", "": "An unnamed app", strings.Repeat("é", 70): strings.Repeat("é", 60)} {
			body, _ := json.Marshal(map[string]any{"client_name": name, "redirect_uris": []string{agentRedirect}})
			_, text := send(a.mux, "POST", "/oauth/register", "application/json", string(body), nil)
			if got := jsonOf(t, text)["client_name"]; got != want {
				t.Errorf("name %q became %q, want %q", name, got, want)
			}
		}
	})
}

func TestAgentConsentPage(t *testing.T) {
	a := newAgentApp(t)
	clientID := a.register(t, "<b>Claude</b> Code", agentRedirect)
	_, challenge := pkce()
	q := authorizeQuery(clientID, agentRedirect, challenge, "st-1")

	t.Run("signed out: our own sign-in first, which comes back here", func(t *testing.T) {
		res, _ := send(a.mux, "GET", "/oauth/authorize?"+q.Encode(), "", "", nil)
		loc, err := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || err != nil || loc.Host != "" || loc.Path != "/auth/github/login" ||
			loc.Query().Get("next") != "/oauth/authorize?"+q.Encode() {
			t.Fatalf("status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("FR-AGENT-2: the page names the app as text, where it sends you back, and who it acts as", func(t *testing.T) {
		res, page := send(a.mux, "GET", "/oauth/authorize?"+q.Encode(), "", "", a.as(t, hostID))
		for _, want := range []string{"&lt;b&gt;Claude&lt;/b&gt; Code", "127.0.0.1", "runs on your computer", "login-gh1", `value="allow"`, "disabled"} {
			if !strings.Contains(page, want) {
				t.Errorf("the page lacks %q", want)
			}
		}
		if strings.Contains(page, "<b>Claude</b>") {
			t.Error("the app's name is markup on the page")
		}
		for header, want := range map[string]string{"X-Frame-Options": "DENY", "Content-Security-Policy": "frame-ancestors 'none'", "Cache-Control": "no-store"} {
			if got := res.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("RFC 8252: a loopback callback on another port is the same callback", func(t *testing.T) {
		other := authorizeQuery(clientID, "http://127.0.0.1:50001/callback", challenge, "st-1")
		if res, page := send(a.mux, "GET", "/oauth/authorize?"+other.Encode(), "", "", a.as(t, hostID)); res.StatusCode != http.StatusOK {
			t.Errorf("another port: status %d:\n%s", res.StatusCode, page)
		}
	})

	t.Run("the resource indicator may differ in case and a trailing slash, or be left out", func(t *testing.T) {
		for _, resource := range []string{"HTTPS://OFFICEHOURS.EXAMPLE/mcp", agentResource + "/", ""} {
			r := authorizeQuery(clientID, agentRedirect, challenge, "st-1")
			r.Set("resource", resource)
			if resource == "" {
				r.Del("resource")
			}
			if res, _ := send(a.mux, "GET", "/oauth/authorize?"+r.Encode(), "", "", a.as(t, hostID)); res.StatusCode != http.StatusOK {
				t.Errorf("resource %q: status %d", resource, res.StatusCode)
			}
		}
	})

	t.Run("OAuth 2.1 §7.12.2: every bad request is a page here, never a redirect to the app", func(t *testing.T) {
		evilClient := a.register(t, "x", "https://evil.example/cb")
		for name, change := range map[string]func(url.Values){
			"an app nobody registered":      func(v url.Values) { v.Set("client_id", "garbage") },
			"a callback it didn't register": func(v url.Values) { v.Set("redirect_uri", "https://evil.example/cb") },
			"another path on loopback":      func(v url.Values) { v.Set("redirect_uri", "http://127.0.0.1:33418/other") },
			"a registered evil callback": func(v url.Values) {
				v.Set("client_id", evilClient)
				v.Set("redirect_uri", "https://evil.example/cb")
				v.Set("response_type", "token")
			},
			"response_type token": func(v url.Values) { v.Set("response_type", "token") },
			"no challenge":        func(v url.Values) { v.Del("code_challenge") },
			"the plain method":    func(v url.Values) { v.Set("code_challenge_method", "plain") },
			"a short challenge":   func(v url.Values) { v.Set("code_challenge", "abc") },
			"another resource":    func(v url.Values) { v.Set("resource", "https://other.example/mcp") },
			"a huge state":        func(v url.Values) { v.Set("state", strings.Repeat("s", 513)) },
		} {
			bad := authorizeQuery(clientID, agentRedirect, challenge, "st-1")
			change(bad)
			for _, cookie := range []*http.Cookie{nil, a.as(t, hostID)} {
				res, page := send(a.mux, "GET", "/oauth/authorize?"+bad.Encode(), "", "", cookie)
				if loc := res.Header.Get("Location"); strings.Contains(loc, "evil.example") || (cookie != nil && loc != "") {
					t.Errorf("%s: redirected to %q", name, loc)
				}
				if cookie != nil && (res.StatusCode != http.StatusBadRequest || !strings.Contains(page, "<!doctype html>")) {
					t.Errorf("%s: status %d, want a 400 page", name, res.StatusCode)
				}
			}
		}
	})
}

func TestAgentDecision(t *testing.T) {
	a := newAgentApp(t)
	clientID := a.register(t, "Claude Code", agentRedirect)
	_, challenge := pkce()
	q := authorizeQuery(clientID, agentRedirect, challenge, "st-xyz")

	t.Run("Allow sends the app a code, its state and our issuer", func(t *testing.T) {
		res := a.decide(a.as(t, hostID), a.consent(t, a.as(t, hostID), q), "allow")
		loc, _ := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || loc.Scheme+"://"+loc.Host+loc.Path != agentRedirect ||
			loc.Query().Get("code") == "" || loc.Query().Get("state") != "st-xyz" || loc.Query().Get("iss") != testSite {
			t.Fatalf("status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
		}
		if len(a.grants.rows) != 0 {
			t.Error("a grant was made before the app turned its code in")
		}
	})

	t.Run("Cancel tells the app no, with its state and our issuer", func(t *testing.T) {
		res := a.decide(a.as(t, hostID), a.consent(t, a.as(t, hostID), q), "deny")
		loc, _ := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || loc.Query().Get("error") != "access_denied" || loc.Query().Get("state") != "st-xyz" ||
			loc.Query().Get("iss") != testSite || loc.Query().Has("code") {
			t.Fatalf("status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("a callback with a query keeps it", func(t *testing.T) {
		withQuery := a.register(t, "x", "https://app.example/cb?tenant=7")
		res := a.decide(a.as(t, hostID), a.consent(t, a.as(t, hostID), authorizeQuery(withQuery, "https://app.example/cb?tenant=7", challenge, "s")), "allow")
		loc, _ := url.Parse(res.Header.Get("Location"))
		if loc.Query().Get("tenant") != "7" || loc.Query().Get("code") == "" {
			t.Fatalf("Location %q", res.Header.Get("Location"))
		}
	})

	t.Run("NFR-SEC-2: another site, another person, no sign-in or a forged form gets no code", func(t *testing.T) {
		form := a.consent(t, a.as(t, hostID), q)
		for name, res := range map[string]*http.Response{
			"another site":     a.decide(a.as(t, hostID), form, "allow", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }),
			"no Origin":        a.decide(a.as(t, hostID), form, "allow", func(r *http.Request) { r.Header.Del("Origin") }),
			"another person":   a.decide(a.as(t, askerID), form, "allow"),
			"signed out":       a.decide(nil, form, "allow"),
			"a forged form":    a.decide(a.as(t, hostID), form+"x", "allow"),
			"a forged cookie":  a.decide(&http.Cookie{Name: session.Cookie, Value: "x"}, form, "allow"),
			"neither decision": a.decide(a.as(t, hostID), form, "maybe"),
		} {
			if res.StatusCode < 400 || res.Header.Get("Location") != "" {
				t.Errorf("%s: status %d, Location %q", name, res.StatusCode, res.Header.Get("Location"))
			}
		}
	})
}

func TestAgentToken(t *testing.T) {
	t.Run("FR-AGENT-2: a code and its verifier buy a bearer token for /mcp, and a grant row", func(t *testing.T) {
		a := newAgentApp(t)
		clientID, verifier, code := a.approved(t, hostID)
		res, text := a.token(exchange(clientID, verifier, code))
		doc := jsonOf(t, text)
		if res.StatusCode != http.StatusOK || doc["token_type"] != "Bearer" || doc["access_token"] == "" || doc["scope"] != "officehours" {
			t.Fatalf("status %d: %v", res.StatusCode, doc)
		}
		if seconds, _ := doc["expires_in"].(float64); seconds != session.Lifetime.Seconds() {
			t.Errorf("expires_in %v, want a web sign-in's %v", doc["expires_in"], session.Lifetime.Seconds())
		}
		if _, has := doc["refresh_token"]; has {
			t.Error("a refresh token was issued")
		}
		if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("headers %v", res.Header)
		}
		if len(a.grants.rows) != 1 {
			t.Fatalf("grants %v", a.grants.rows)
		}
		for _, g := range a.grants.rows {
			if g.UserID != hostID || g.ClientName != "Claude Code" || g.RedirectHost != "127.0.0.1" {
				t.Errorf("grant %+v", g)
			}
		}
		var tok agentToken
		if err := a.codec.Open(accessSeal, doc["access_token"].(string), &tok); err != nil || tok.User.ID != hostID ||
			tok.User.GitHubToken != testUserToken || tok.Audience != agentResource || a.grants.rows[tok.GrantID].UserID != hostID {
			t.Errorf("the token holds %+v (%v)", tok, err)
		}
	})

	t.Run("RFC 6749 §4.1.2: a code turned in twice is refused, and its grant ends", func(t *testing.T) {
		a := newAgentApp(t)
		clientID, verifier, code := a.approved(t, hostID)
		a.token(exchange(clientID, verifier, code))
		res, text := a.token(exchange(clientID, verifier, code))
		if res.StatusCode != http.StatusBadRequest || jsonOf(t, text)["error"] != "invalid_grant" {
			t.Fatalf("second exchange: status %d: %s", res.StatusCode, text)
		}
		for id := range a.grants.rows {
			if !a.grants.revoked[id] {
				t.Error("the replayed code's grant still stands")
			}
		}
	})

	t.Run("a wrong verifier, app, callback or resource is refused before the code is used up", func(t *testing.T) {
		a := newAgentApp(t)
		clientID, verifier, code := a.approved(t, hostID)
		otherClient := a.register(t, "x", agentRedirect)
		for name, c := range map[string]struct {
			change func(url.Values)
			err    string
		}{
			"wrong verifier":     {func(v url.Values) { v.Set("code_verifier", strings.Repeat("a", 43)) }, "invalid_grant"},
			"no verifier":        {func(v url.Values) { v.Del("code_verifier") }, "invalid_grant"},
			"another app":        {func(v url.Values) { v.Set("client_id", otherClient) }, "invalid_grant"},
			"another callback":   {func(v url.Values) { v.Set("redirect_uri", "http://127.0.0.1:33418/other") }, "invalid_grant"},
			"another resource":   {func(v url.Values) { v.Set("resource", "https://other.example/mcp") }, "invalid_target"},
			"a forged code":      {func(v url.Values) { v.Set("code", code+"x") }, "invalid_grant"},
			"no code":            {func(v url.Values) { v.Del("code") }, "invalid_request"},
			"a refresh token":    {func(v url.Values) { v.Set("grant_type", "refresh_token") }, "unsupported_grant_type"},
			"client credentials": {func(v url.Values) { v.Set("grant_type", "client_credentials") }, "unsupported_grant_type"},
			"an access token":    {func(v url.Values) { v.Set("code", a.connect(t, askerID)) }, "invalid_grant"},
			"a consent form": {func(v url.Values) {
				v.Set("code", a.consent(t, a.as(t, hostID), authorizeQuery(clientID, agentRedirect, "x"+strings.Repeat("A", 42), "s")))
			}, "invalid_grant"},
			"a session cookie":    {func(v url.Values) { v.Set("code", a.as(t, hostID).Value) }, "invalid_grant"},
			"a sealed client id":  {func(v url.Values) { v.Set("code", clientID) }, "invalid_grant"},
			"an unknown verifier": {func(v url.Values) { v.Set("code_verifier", "short") }, "invalid_grant"},
		} {
			form := exchange(clientID, verifier, code)
			c.change(form)
			res, text := a.token(form)
			if res.StatusCode != http.StatusBadRequest || jsonOf(t, text)["error"] != c.err {
				t.Errorf("%s: status %d: %s", name, res.StatusCode, text)
			}
		}
		before := len(a.grants.rows)
		if res, text := a.token(exchange(clientID, verifier, code)); res.StatusCode != http.StatusOK {
			t.Fatalf("the real exchange after the refusals: status %d: %s", res.StatusCode, text)
		}
		if len(a.grants.rows) != before+1 {
			t.Error("the refusals made grants")
		}
	})

	t.Run("the app may name itself with Basic auth instead", func(t *testing.T) {
		a := newAgentApp(t)
		clientID, verifier, code := a.approved(t, hostID)
		form := exchange(clientID, verifier, code)
		form.Del("client_id")
		res, text := a.token(form, func(r *http.Request) { r.SetBasicAuth(url.QueryEscape(clientID), "") })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, text)
		}
	})

	t.Run("a code past its five minutes is refused", func(t *testing.T) {
		a := newAgentApp(t)
		clientID := a.register(t, "x", agentRedirect)
		verifier, challenge := pkce()
		old, err := a.codec.Seal(codeSeal, authCode{GrantID: "g1", User: session.User{ID: hostID}, Client: clientHash(clientID), RedirectURI: agentRedirect, Challenge: challenge, Resource: agentResource}, -time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if res, text := a.token(exchange(clientID, verifier, old)); res.StatusCode != http.StatusBadRequest || jsonOf(t, text)["error"] != "invalid_grant" {
			t.Fatalf("status %d: %s", res.StatusCode, text)
		}
	})

	t.Run("a store that fails is a server error that says nothing of why", func(t *testing.T) {
		a := newAgentApp(t)
		clientID, verifier, code := a.approved(t, hostID)
		a.grants.err = errors.New("dial tcp 10.0.0.7:5432: connection reset")
		res, text := a.token(exchange(clientID, verifier, code))
		if res.StatusCode != http.StatusInternalServerError || jsonOf(t, text)["error"] != "server_error" || strings.Contains(text, "10.0.0.7") {
			t.Fatalf("status %d: %s", res.StatusCode, text)
		}
	})
}

func TestConnectedAgents(t *testing.T) {
	a := newAgentApp(t)
	a.connect(t, hostID)
	var id string
	for gid := range a.grants.rows {
		id = gid
	}

	t.Run("FR-AGENT-3: a person sees the agents they connected", func(t *testing.T) {
		if res, _ := call(a.mux, "GET", "/api/me/agents", "", nil); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("signed out: status %d", res.StatusCode)
		}
		res, body := call(a.mux, "GET", "/api/me/agents", "", a.as(t, hostID))
		var env struct {
			OK   bool `json:"ok"`
			Data []struct {
				ID, App, ReturnsTo, ConnectedAt string
				LastUsedAt                      *string
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil || res.StatusCode != http.StatusOK || len(env.Data) != 1 ||
			env.Data[0].ID != id || env.Data[0].App != "Claude Code" || env.Data[0].ReturnsTo != "127.0.0.1" || env.Data[0].ConnectedAt == "" {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if _, others := call(a.mux, "GET", "/api/me/agents", "", a.as(t, askerID)); strings.Contains(others, id) {
			t.Error("another person sees this person's agent")
		}
	})

	t.Run("FR-AGENT-3: Disconnect ends it, for its own person only", func(t *testing.T) {
		if res, _ := call(a.mux, "POST", "/api/me/agents/"+id+"/disconnect", "{}", a.as(t, askerID)); res.StatusCode != http.StatusNotFound || a.grants.revoked[id] {
			t.Fatalf("another person: status %d, revoked %v", res.StatusCode, a.grants.revoked[id])
		}
		if res, _ := call(a.mux, "POST", "/api/me/agents/"+id+"/disconnect", "{}", a.as(t, hostID), func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); res.StatusCode != http.StatusForbidden {
			t.Fatalf("another site: status %d", res.StatusCode)
		}
		if res, body := call(a.mux, "POST", "/api/me/agents/"+id+"/disconnect", "{}", a.as(t, hostID)); res.StatusCode != http.StatusOK || !a.grants.revoked[id] {
			t.Fatalf("its person: status %d: %s", res.StatusCode, body)
		}
		if _, body := call(a.mux, "GET", "/api/me/agents", "", a.as(t, hostID)); strings.Contains(body, id) {
			t.Error("a disconnected agent is still listed")
		}
	})
}

func TestAgentSignInLogsNoSecret(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	a := newAgentApp(t)
	clientID, verifier, code := a.approved(t, hostID)
	_, text := a.token(exchange(clientID, verifier, code))
	access := jsonOf(t, text)["access_token"].(string)
	a.token(exchange(clientID, verifier, code)) // a replay, which is logged
	a.grants.err = errors.New("connection reset")
	_, _, code2 := a.approved(t, hostID)
	a.token(exchange(clientID, verifier, code2)) // a failure, which is logged

	logged := buf.String()
	if logged == "" {
		t.Fatal("nothing was logged: the replay and the failure should be")
	}
	for name, secret := range map[string]string{"code": code, "second code": code2, "verifier": verifier, "access token": access, "GitHub token": testUserToken} {
		if strings.Contains(logged, secret) {
			t.Errorf("NFR-SEC-1: the %s is in the log:\n%s", name, logged)
		}
	}
}

func TestAgentSignInRoundTripsThroughGitHub(t *testing.T) {
	a := newAuthApp(t)
	// The largest request we accept, carried through GitHub's sign-in in the state cookie.
	redirects := []string{
		"https://a.example/" + strings.Repeat("x", 230), "https://b.example/" + strings.Repeat("y", 230), "https://c.example/" + strings.Repeat("z", 230),
	}
	clientID, err := a.codec.Seal(clientSeal, clientInfo{Name: strings.Repeat("n", 60), Redirects: redirects}, clientTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, challenge := pkce()
	next := "/oauth/authorize?" + authorizeQuery(clientID, redirects[2], challenge, strings.Repeat("s", 512)).Encode()
	_, stateCookie := a.startLogin(t, next)
	if size := len((&http.Cookie{Name: stateCookie.Name, Value: stateCookie.Value, Path: "/", MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}).String()); size > 4000 {
		t.Fatalf("the sign-in cookie is %d bytes: a browser drops one over 4096", size)
	}
	if res := a.signIn(t, next); res.StatusCode != http.StatusFound || res.Header.Get("Location") != next {
		t.Fatalf("after GitHub: status %d, Location %q, want back on the consent page", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestWhereAnAppSendsYouBack(t *testing.T) {
	for uri, want := range map[string]string{
		"http://127.0.0.1:33418/callback":              "127.0.0.1",
		"https://claude.ai/api/mcp/auth_callback":      "claude.ai",
		"cursor://anysphere.cursor-mcp/oauth/callback": "cursor://anysphere.cursor-mcp",
		"com.example.app:/oauth":                       "com.example.app:",
	} {
		if got := returnsTo(uri); got != want {
			t.Errorf("returnsTo(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestAgentsWithoutADatabase(t *testing.T) {
	a := newAgentApp(t)
	a.deps.grants = nil
	a.remux()
	if res, page := send(a.mux, "GET", "/oauth/authorize?client_id=x", "", "", a.as(t, hostID)); res.StatusCode != http.StatusServiceUnavailable || !strings.Contains(page, "can&#39;t connect") {
		t.Errorf("authorize: status %d", res.StatusCode)
	}
	if res, text := a.token(url.Values{"grant_type": {"authorization_code"}}); res.StatusCode != http.StatusServiceUnavailable || jsonOf(t, text)["error"] != "temporarily_unavailable" {
		t.Errorf("token: status %d: %s", res.StatusCode, text)
	}
	if res, _ := call(a.mux, "GET", "/api/me/agents", "", a.as(t, hostID)); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("list: status %d", res.StatusCode)
	}
	if res, _ := call(a.mux, "POST", "/api/me/agents/g1/disconnect", "{}", a.as(t, hostID)); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("disconnect: status %d", res.StatusCode)
	}
}
