package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"officehours/github"
	"officehours/guard"
	"officehours/session"
	"officehours/store"
)

const (
	testSite = "https://officehours.example"
	testHost = "officehours.example"
	// Stand-ins, joined at run time so CI's secret scan stays strict about real ones.
	testClientSecret = "test-client-" + "secret-value"
	testUserToken    = "gho_" + "fake_user_token_1234567890"
)

// fakeGitHubAPI answers the token and user endpoints and remembers what it was sent.
type fakeGitHubAPI struct {
	*httptest.Server
	tokenReply string
	userReply  string
	userStatus int
	gotForm    url.Values
}

func newFakeGitHubAPI(t *testing.T) *fakeGitHubAPI {
	t.Helper()
	f := &fakeGitHubAPI{
		tokenReply: `{"access_token":"` + testUserToken + `","scope":"read:user","token_type":"bearer"}`,
		userReply:  `{"id":42,"login":"mia-k","name":"Mia K","avatar_url":"https://avatars.example/u/42","email":"mia@example.com"}`,
		userStatus: 200,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, f.tokenReply)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.userStatus)
		io.WriteString(w, f.userReply)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

type fakeUsers struct {
	saved []store.User
	err   error
}

func (f *fakeUsers) UpsertUser(_ context.Context, u store.User) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, u)
	return nil
}

// authApp is the server with a fake GitHub behind it and an in-memory user table.
type authApp struct {
	mux   *http.ServeMux
	gh    *fakeGitHubAPI
	users *fakeUsers
	codec *session.Codec
}

func newAuthApp(t *testing.T) *authApp {
	t.Helper()
	codec, err := session.NewCodec(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	g, err := guard.New(testSite, codec, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	gh := newFakeGitHubAPI(t)
	client, err := github.New(github.Config{
		ClientID: "Iv1.test-client-id", ClientSecret: testClientSecret,
		RedirectURL: testSite + "/auth/github/callback",
		AuthURL:     gh.URL + "/login/oauth/authorize", TokenURL: gh.URL + "/login/oauth/access_token", APIURL: gh.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("github.New: %v", err)
	}
	users := &fakeUsers{}
	mux := newMux(config{AppURL: testSite}, deps{guard: g, sessions: codec, github: client, users: users})
	return &authApp{mux: mux, gh: gh, users: users, codec: codec}
}

// do sends a request as a browser on our own site would, with the given cookies.
func (a *authApp) do(method, target string, cookies []*http.Cookie, mutate ...func(*http.Request)) *http.Response {
	var body io.Reader
	if method == "POST" {
		body = strings.NewReader(`{}`)
	}
	r := httptest.NewRequest(method, target, body)
	r.Host = testHost
	if method == "POST" {
		r.Header.Set("Origin", testSite)
		r.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for _, m := range mutate {
		m(r)
	}
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, r)
	return rec.Result()
}

func cookieNamed(res *http.Response, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// startLogin runs /auth/github/login and returns GitHub's authorize URL and the state cookie.
func (a *authApp) startLogin(t *testing.T, next string) (*url.URL, *http.Cookie) {
	t.Helper()
	target := "/auth/github/login"
	if next != "" {
		target += "?next=" + url.QueryEscape(next)
	}
	res := a.do("GET", target, nil)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login: status %d, want 302", res.StatusCode)
	}
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("login: bad Location: %v", err)
	}
	c := cookieNamed(res, oauthCookie)
	if c == nil {
		t.Fatal("login: no state cookie")
	}
	return loc, c
}

// signIn runs the whole flow and returns the callback's response.
func (a *authApp) signIn(t *testing.T, next string) *http.Response {
	t.Helper()
	loc, c := a.startLogin(t, next)
	return a.do("GET", "/auth/github/callback?code=code-xyz&state="+loc.Query().Get("state"), []*http.Cookie{c})
}

func TestLoginSendsTheBrowserToGitHub(t *testing.T) {
	a := newAuthApp(t)
	loc, c := a.startLogin(t, "")
	q := loc.Query()
	if !strings.HasPrefix(loc.String(), a.gh.URL+"/login/oauth/authorize?") {
		t.Fatalf("FR-AUTH-1: redirected to %s, want GitHub's authorize page", loc)
	}
	if q.Get("scope") != "read:user" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || len(q.Get("state")) < 20 {
		t.Fatalf("FR-AUTH-1 / NFR-SEC-5: authorize parameters = %v", q)
	}
	// The callback address comes from APP_URL, never from the request's Host header.
	if q.Get("redirect_uri") != testSite+"/auth/github/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 600 {
		t.Fatalf("NFR-SEC-5: state cookie = %+v, want HttpOnly, Secure, SameSite=Lax, 10 minutes", c)
	}
	if strings.Contains(c.Value, q.Get("state")) {
		t.Fatal("NFR-SEC-5: the state cookie shows the state in the clear")
	}
	// Two logins never share a state or a challenge.
	loc2, _ := a.startLogin(t, "")
	if loc2.Query().Get("state") == q.Get("state") || loc2.Query().Get("code_challenge") == q.Get("code_challenge") {
		t.Fatal("NFR-SEC-5: two logins got the same state or challenge")
	}
}

func TestLoginOnAnotherHostGoesToAppURLFirst(t *testing.T) {
	a := newAuthApp(t)
	// Every Vercel deployment also answers on its own URL; a state cookie set there would be lost.
	res := a.do("GET", "/auth/github/login?next=%2Fp%2Fpaperplane%2Fstash", nil, func(r *http.Request) { r.Host = "office-hours-abc123.vercel.app" })
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil || res.StatusCode != http.StatusFound || loc.Scheme+"://"+loc.Host != testSite ||
		loc.Path != "/auth/github/login" || loc.Query().Get("next") != "/p/paperplane/stash" {
		t.Fatalf("status %d, Location %q, want a redirect to APP_URL that keeps next", res.StatusCode, res.Header.Get("Location"))
	}
	if cookieNamed(res, oauthCookie) != nil {
		t.Fatal("a state cookie was set on the wrong host")
	}
}

func TestLoginBehindAProxyDoesNotLoop(t *testing.T) {
	a := newAuthApp(t)
	toGitHub := func(res *http.Response) bool {
		return res.StatusCode == http.StatusFound && strings.HasPrefix(res.Header.Get("Location"), a.gh.URL+"/login/oauth/authorize?")
	}

	// A proxy in front (Vercel's router, the Vite dev server) may hand us its own Host and put
	// the browser's in X-Forwarded-Host. That is our site, so the sign-in starts.
	res := a.do("GET", "/auth/github/login", nil, func(r *http.Request) {
		r.Host = "localhost:8080"
		r.Header.Set("X-Forwarded-Host", testHost)
	})
	if !toGitHub(res) {
		t.Fatalf("with X-Forwarded-Host = our host: status %d, Location %q, want GitHub", res.StatusCode, res.Header.Get("Location"))
	}

	// If the browser's host can't be seen at all, the first request is sent to APP_URL once...
	res = a.do("GET", "/auth/github/login?next=%2Fp%2Fpaperplane%2Fstash", nil, func(r *http.Request) { r.Host = "internal-name:8080" })
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil || res.StatusCode != http.StatusFound || loc.Scheme+"://"+loc.Host != testSite || loc.Path != "/auth/github/login" {
		t.Fatalf("first hop: status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
	}
	// ...and the request that comes back from that redirect is served, whatever Host it shows.
	// Without this the browser would be bounced forever.
	res = a.do("GET", loc.RequestURI(), nil, func(r *http.Request) { r.Host = "internal-name:8080" })
	if !toGitHub(res) {
		t.Fatalf("second hop: status %d, Location %q, want GitHub (a redirect loop otherwise)", res.StatusCode, res.Header.Get("Location"))
	}
	// The page to land on survives the hop.
	var st oauthState
	if err := a.codec.Open(oauthCookie, cookieNamed(res, oauthCookie).Value, &st); err != nil || st.Next != "/p/paperplane/stash" {
		t.Fatalf("after the hop: state %+v, err %v, want next=/p/paperplane/stash", st, err)
	}
}

func TestNextIsOnlyEverOneOfOurOwnPages(t *testing.T) {
	for next, want := range map[string]string{
		"":                           "/dashboard",
		"/dashboard":                 "/dashboard",
		"/p/paperplane/stash":        "/p/paperplane/stash",
		"/p/Paper-Plane/My.Repo_1":   "/p/Paper-Plane/My.Repo_1",
		"//evil.example":             "/dashboard",
		`/\evil.example`:             "/dashboard",
		"https://evil.example":       "/dashboard",
		"https://evil.example/p/a/b": "/dashboard",
		"/p/paperplane/stash/faq":    "/dashboard",
		"/p/paperplane/..":           "/dashboard",
		"/p/paperplane/.":            "/dashboard",
		"/p/paperplane/stash?x=1":    "/dashboard",
		"/p/paperplane/stash#x":      "/dashboard",
		"/p/paperplane":              "/dashboard",
		"/dashboard/../admin":        "/dashboard",
		"javascript:alert(1)":        "/dashboard",
	} {
		t.Run(next, func(t *testing.T) {
			a := newAuthApp(t)
			res := a.signIn(t, next)
			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != want {
				t.Fatalf("NFR-SEC-5: after sign-in with next=%q: status %d, Location %q, want %q", next, res.StatusCode, res.Header.Get("Location"), want)
			}
		})
	}
}

func TestCallbackSignsTheUserIn(t *testing.T) {
	a := newAuthApp(t)
	loc, stateCookie := a.startLogin(t, "/p/paperplane/stash")
	res := a.do("GET", "/auth/github/callback?code=code-xyz&state="+loc.Query().Get("state"), []*http.Cookie{stateCookie})

	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/p/paperplane/stash" {
		t.Fatalf("status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
	}
	// NFR-SEC-5: the verifier GitHub received is the one whose hash went out in the redirect.
	if got := github.Challenge(a.gh.gotForm.Get("code_verifier")); got != loc.Query().Get("code_challenge") {
		t.Fatalf("the verifier sent to GitHub does not match the challenge: %q vs %q", got, loc.Query().Get("code_challenge"))
	}
	if a.gh.gotForm.Get("code") != "code-xyz" {
		t.Fatalf("code sent to GitHub = %q", a.gh.gotForm.Get("code"))
	}

	// FR-AUTH-1, NFR-PRIV-1: the stored user has id, login, name and avatar. There is no email field.
	if len(a.users.saved) != 1 || a.users.saved[0] != (store.User{ID: "gh42", GitHubID: 42, Login: "mia-k", Name: "Mia K", AvatarURL: "https://avatars.example/u/42"}) {
		t.Fatalf("saved users = %+v", a.users.saved)
	}

	sess := cookieNamed(res, session.Cookie)
	if sess == nil || !sess.HttpOnly || !sess.Secure || sess.SameSite != http.SameSiteLaxMode || sess.MaxAge != int(session.Lifetime/time.Second) {
		t.Fatalf("NFR-SEC-5: session cookie = %+v", sess)
	}
	if strings.Contains(sess.Value, testUserToken) {
		t.Fatal("FR-AUTH-1: the GitHub token is readable in the cookie")
	}
	var u session.User
	if err := a.codec.Open(session.Cookie, sess.Value, &u); err != nil {
		t.Fatalf("open session: %v", err)
	}
	if u != (session.User{ID: "gh42", Login: "mia-k", Name: "Mia K", AvatarURL: "https://avatars.example/u/42", GitHubToken: testUserToken}) {
		t.Fatalf("session = %+v", u)
	}

	// The state cookie is used up, and nothing under /auth may be cached.
	if used := cookieNamed(res, oauthCookie); used == nil || used.MaxAge >= 0 {
		t.Fatalf("NFR-SEC-5: the state cookie was not cleared: %+v", used)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", res.Header.Get("Cache-Control"))
	}
}

func TestNobodyIsCalledLikeTheBot(t *testing.T) {
	// A GitHub name is free text. In the chat, someone called "Office Hours" would look like the
	// sender of the cards, so that one name falls back to the login.
	for _, name := range []string{"Office Hours", "  office hours ", "OFFICE HOURS"} {
		a := newAuthApp(t)
		a.gh.userReply = `{"id":7,"login":"quiet-dev","name":"` + name + `","avatar_url":""}`
		res := a.signIn(t, "")
		var u session.User
		if err := a.codec.Open(session.Cookie, cookieNamed(res, session.Cookie).Value, &u); err != nil {
			t.Fatalf("open session: %v", err)
		}
		if u.Name != "quiet-dev" || len(a.users.saved) != 1 || a.users.saved[0].Name != "quiet-dev" {
			t.Errorf("GitHub name %q: session %q, saved %+v; want the login", name, u.Name, a.users.saved)
		}
	}
}

func TestCallbackWithoutANameUsesTheLogin(t *testing.T) {
	a := newAuthApp(t)
	a.gh.userReply = `{"id":7,"login":"quiet-dev","name":null,"avatar_url":""}`
	res := a.signIn(t, "")
	var u session.User
	if err := a.codec.Open(session.Cookie, cookieNamed(res, session.Cookie).Value, &u); err != nil {
		t.Fatalf("open session: %v", err)
	}
	if u.Name != "quiet-dev" || len(a.users.saved) != 1 || a.users.saved[0].Name != "quiet-dev" {
		t.Fatalf("FR-PROJ-4 needs a name on every profile: session %q, saved %+v", u.Name, a.users.saved)
	}
}

func TestCallbackRefusals(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(a *authApp, loc *url.URL, c *http.Cookie) (target string, cookies []*http.Cookie)
		want   string // the reason in /?signin=
	}{
		{"NFR-SEC-5: a state that is not ours", func(a *authApp, _ *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			return "/auth/github/callback?code=code-xyz&state=someone-elses", []*http.Cookie{c}
		}, "expired"},
		{"NFR-SEC-5: no state in the query", func(a *authApp, _ *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			return "/auth/github/callback?code=code-xyz", []*http.Cookie{c}
		}, "expired"},
		{"NFR-SEC-5: no state cookie (another browser, or it was used already)", func(a *authApp, loc *url.URL, _ *http.Cookie) (string, []*http.Cookie) {
			return "/auth/github/callback?code=code-xyz&state=" + loc.Query().Get("state"), nil
		}, "expired"},
		{"NFR-SEC-5: a state cookie past its ten minutes", func(a *authApp, _ *url.URL, _ *http.Cookie) (string, []*http.Cookie) {
			old, _ := a.codec.Seal(oauthCookie, oauthState{State: "old-state", Verifier: "v", Next: "/dashboard"}, -time.Minute)
			return "/auth/github/callback?code=code-xyz&state=old-state", []*http.Cookie{{Name: oauthCookie, Value: old}}
		}, "expired"},
		{"NFR-SEC-5: a session cookie sent as the state cookie", func(a *authApp, _ *url.URL, _ *http.Cookie) (string, []*http.Cookie) {
			forged, _ := a.codec.Seal(session.Cookie, oauthState{State: "s", Verifier: "v", Next: "/dashboard"}, time.Hour)
			return "/auth/github/callback?code=code-xyz&state=s", []*http.Cookie{{Name: oauthCookie, Value: forged}}
		}, "expired"},
		{"the user pressed Cancel on GitHub", func(a *authApp, loc *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			return "/auth/github/callback?error=access_denied&error_description=The+user+has+denied+access&state=" + loc.Query().Get("state"), []*http.Cookie{c}
		}, "cancelled"},
		{"no code in the query", func(a *authApp, loc *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			return "/auth/github/callback?state=" + loc.Query().Get("state"), []*http.Cookie{c}
		}, "github"},
		{"GitHub refuses the code", func(a *authApp, loc *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			a.gh.tokenReply = `{"error":"bad_verification_code"}`
			return "/auth/github/callback?code=code-xyz&state=" + loc.Query().Get("state"), []*http.Cookie{c}
		}, "github"},
		{"the code is fine but reading the user fails", func(a *authApp, loc *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			a.gh.userStatus = 500
			return "/auth/github/callback?code=code-xyz&state=" + loc.Query().Get("state"), []*http.Cookie{c}
		}, "github"},
		{"our database is down", func(a *authApp, loc *url.URL, c *http.Cookie) (string, []*http.Cookie) {
			a.users.err = errors.New("connection refused")
			return "/auth/github/callback?code=code-xyz&state=" + loc.Query().Get("state"), []*http.Cookie{c}
		}, "server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAuthApp(t)
			loc, c := a.startLogin(t, "/p/paperplane/stash")
			target, cookies := tt.break_(a, loc, c)
			res := a.do("GET", target, cookies)
			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/?signin="+tt.want {
				t.Fatalf("status %d, Location %q, want /?signin=%s", res.StatusCode, res.Header.Get("Location"), tt.want)
			}
			if cookieNamed(res, session.Cookie) != nil {
				t.Fatal("a session cookie was set on a failed sign-in")
			}
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", res.Header.Get("Cache-Control"))
			}
		})
	}
}

func TestSignInWithoutSetupSaysSo(t *testing.T) {
	codec, _ := session.NewCodec(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	g, _ := guard.New(testSite, codec, nil)
	mux := newMux(config{AppURL: testSite}, deps{guard: g, sessions: codec}) // no GitHub app, no database
	for _, target := range []string{"/auth/github/login", "/auth/github/callback?code=x&state=y"} {
		r := httptest.NewRequest("GET", target, nil)
		r.Host = testHost
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?signin=setup" {
			t.Errorf("%s: status %d, Location %q, want /?signin=setup", target, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestNothingSecretReachesTheLog(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// One sign-in that works and one of every kind that fails after GitHub was called.
	a := newAuthApp(t)
	a.signIn(t, "/dashboard")
	a.gh.tokenReply = `{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`
	a.signIn(t, "/dashboard")
	a = newAuthApp(t)
	a.gh.userStatus = 401
	a.signIn(t, "/dashboard")
	a = newAuthApp(t)
	a.users.err = errors.New("connection refused")
	a.signIn(t, "/dashboard")

	logged := buf.String()
	if logged == "" {
		t.Fatal("the failures were not logged at all (NFR-OPS-2 wants them logged)")
	}
	verifier := a.gh.gotForm.Get("code_verifier")
	for name, secret := range map[string]string{"client secret": testClientSecret, "user token": testUserToken, "code": "code-xyz", "verifier": verifier} {
		if secret != "" && strings.Contains(logged, secret) {
			t.Errorf("NFR-SEC-1: the %s is in the log:\n%s", name, logged)
		}
	}
}

func TestMe(t *testing.T) {
	a := newAuthApp(t)
	if res := a.do("GET", "/api/me", nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out: status %d, want 401", res.StatusCode)
	}
	sess := cookieNamed(a.signIn(t, ""), session.Cookie)
	res := a.do("GET", "/api/me", []*http.Cookie{sess})
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("signed in: status %d: %s", res.StatusCode, body)
	}
	for _, want := range []string{`"id":"gh42"`, `"login":"mia-k"`, `"name":"Mia K"`, `"avatarUrl":"https://avatars.example/u/42"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("/api/me = %s, missing %s", body, want)
		}
	}
	// FR-AUTH-1: the GitHub token never reaches client JavaScript.
	if strings.Contains(string(body), testUserToken) || strings.Contains(string(body), "tok") {
		t.Fatalf("FR-AUTH-1: /api/me exposes the GitHub token: %s", body)
	}
}

func TestLogout(t *testing.T) {
	a := newAuthApp(t)
	sess := cookieNamed(a.signIn(t, ""), session.Cookie)

	// Another site can't sign the user out (NFR-SEC-5).
	res := a.do("POST", "/auth/logout", []*http.Cookie{sess}, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	if res.StatusCode != http.StatusForbidden || cookieNamed(res, session.Cookie) != nil {
		t.Fatalf("cross-site logout: status %d, cookie %+v", res.StatusCode, cookieNamed(res, session.Cookie))
	}

	// FR-AUTH-4: our own page can, and the cookie is expired with the attributes it was set with.
	res = a.do("POST", "/auth/logout", []*http.Cookie{sess})
	cleared := cookieNamed(res, session.Cookie)
	if res.StatusCode != http.StatusOK || cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" || !cleared.Secure || !cleared.HttpOnly {
		t.Fatalf("logout: status %d, cookie %+v", res.StatusCode, cleared)
	}
	// Signing out twice is not an error.
	if res := a.do("POST", "/auth/logout", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("logout when signed out: status %d", res.StatusCode)
	}
}

func TestNewDepsRefusesToStartWithoutItsTwoSecrets(t *testing.T) {
	goodKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	for _, k := range []string{"DATABASE_URL", "AUTH_GITHUB_ID", "AUTH_GITHUB_SECRET", "NEXT_PUBLIC_NEXCONN_APP_KEY", "NEXCONN_APP_SECRET", "NEXCONN_API_BASE_URL"} {
		t.Setenv(k, "")
	}

	// NFR-SEC-5: no AUTH_SECRET, or one of the wrong length, and the server does not start.
	for name, key := range map[string]string{"missing": "", "too short": base64.StdEncoding.EncodeToString([]byte("short")), "not base64": "%%%"} {
		t.Setenv("AUTH_SECRET", key)
		if _, err := newDeps(context.Background(), config{AppURL: testSite}); err == nil {
			t.Errorf("AUTH_SECRET %s: newDeps started anyway", name)
		}
	}
	// The Origin check and the OAuth redirect compare against APP_URL, so it must be a plain origin.
	t.Setenv("AUTH_SECRET", goodKey)
	for _, appURL := range []string{"", "officehours.example", "https://officehours.example/app"} {
		if _, err := newDeps(context.Background(), config{AppURL: appURL}); err == nil {
			t.Errorf("APP_URL %q: newDeps started anyway", appURL)
		}
	}

	// With the two of them, the server starts; sign-in is off until the GitHub app and the
	// database are configured.
	d, err := newDeps(context.Background(), config{AppURL: testSite})
	if err != nil {
		t.Fatalf("newDeps: %v", err)
	}
	if d.guard == nil || d.sessions == nil || d.github != nil || d.users != nil || d.db != nil {
		t.Fatalf("deps without optional settings = %+v", d)
	}
	// These are interfaces: a nil pointer put into one would not compare equal to nil, and the
	// "not set up" answers of the routes would silently stop working.
	if d.projects != nil || d.rooms != nil || d.tokens != nil {
		t.Fatalf("an optional part is not a nil interface: projects %v, rooms %v, tokens %v", d.projects != nil, d.rooms != nil, d.tokens != nil)
	}

	t.Setenv("AUTH_GITHUB_ID", "Iv1.test-client-id")
	t.Setenv("AUTH_GITHUB_SECRET", testClientSecret)
	t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/db") // well formed; nothing connects yet
	d, err = newDeps(context.Background(), config{AppURL: testSite})
	if err != nil {
		t.Fatalf("newDeps with everything: %v", err)
	}
	if d.github == nil || d.users == nil || d.db == nil || d.projects == nil {
		t.Fatalf("deps with everything = %+v", d)
	}
	t.Setenv("NEXT_PUBLIC_NEXCONN_APP_KEY", "pk_test")
	t.Setenv("NEXCONN_APP_SECRET", "secret_test")
	t.Setenv("NEXCONN_API_BASE_URL", "https://api.example")
	if d, err = newDeps(context.Background(), config{AppURL: testSite}); err != nil || d.rooms == nil || d.tokens == nil {
		t.Fatalf("deps with Nexconn configured: rooms %v, tokens %v, err %v", d.rooms != nil, d.tokens != nil, err)
	}
	// The callback address GitHub is given comes from APP_URL.
	if got := d.github.AuthorizeURL("s", "v"); !strings.Contains(got, url.QueryEscape(testSite+"/auth/github/callback")) {
		t.Fatalf("redirect_uri is not built from APP_URL: %s", got)
	}
}
