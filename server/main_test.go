package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDB stands in for the connection pool.
type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestHealthReportsTheDatabase(t *testing.T) {
	tests := []struct {
		name string
		db   pinger
		want string
	}{
		{"no database configured", nil, `"db":"off"`},
		{"the database answers", fakeDB{}, `"db":"up"`},
		{"NFR-REL-1: an unreachable database is reported, not hidden", fakeDB{err: errors.New("connection refused")}, `"db":"down"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newMux(config{}, deps{db: tt.db}).ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), tt.want) || !strings.Contains(rec.Body.String(), `"ok":true`) {
				t.Fatalf("status %d body %q, want 200 with ok and %s", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestRoutes(t *testing.T) {
	full := config{AppKey: "pk_test", Area: "sg", AppURL: "http://localhost:5173"}
	tests := []struct {
		name       string
		cfg        config
		method     string
		path       string
		wantStatus int
		wantBody   string // substring; "" skips the check
	}{
		{"health answers ok", full, "GET", "/api/health", 200, `"ok":true`},
		{"config returns the public client config", full, "GET", "/api/config", 200, `"appKey":"pk_test","area":"sg"`},
		{"config says so when the server has no Nexconn config", config{}, "GET", "/api/config", 503, `"ok":false`},
		{"unknown API path is 404", full, "GET", "/api/nope", 404, ""},
		{"wrong method is refused", full, "POST", "/api/health", 405, ""},
		{"FR-FAQ-3: FAQ list route reaches Go", full, "GET", "/p/paperplane/stash/faq", 200, "paperplane/stash"},
		{"FR-FAQ-3: FAQ entry route reaches Go", full, "GET", "/p/paperplane/stash/faq/retry-on-5xx", 200, "retry-on-5xx"},
		{"NFR-SEC-3: owner with a bad character is 404", full, "GET", "/p/bad_owner/stash/faq", 404, ""},
		{"NFR-SEC-3: owner longer than 39 is 404", full, "GET", "/p/" + strings.Repeat("a", 40) + "/stash/faq", 404, ""},
		{"NFR-SEC-3: repo with markup is 404", full, "GET", "/p/a/%3Cb%3E/faq", 404, ""},
		{"NFR-SEC-3: slug with markup is 404", full, "GET", "/p/a/b/faq/%3Cscript%3E", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newMux(tt.cfg, deps{}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			if strings.Contains(rec.Body.String(), "<script>") || strings.Contains(rec.Body.String(), "<b>") {
				t.Fatalf("NFR-SEC-6: user text reached the page unescaped: %q", rec.Body.String())
			}
		})
	}
}

func TestValidOwnerRepo(t *testing.T) {
	tests := []struct {
		owner, repo string
		want        bool
	}{
		{"paperplane", "stash", true},
		{"a-b", "My.Repo_1-x", true},
		{strings.Repeat("a", 39), strings.Repeat("r", 100), true},
		{"", "stash", false},
		{"paperplane", "", false},
		{strings.Repeat("a", 40), "stash", false},
		{"paperplane", strings.Repeat("r", 101), false},
		{"under_score", "stash", false},
		{"paperplane", "sta sh", false},
		{"paperplane", "st/ash", false},
		{"paperplane", ".", false}, // GitHub has no such repos, and as path parts they mean "here" and "up"
		{"paperplane", "..", false},
		{"paperplane", ".github", true},
	}
	for _, tt := range tests {
		if got := validOwnerRepo(tt.owner, tt.repo); got != tt.want {
			t.Errorf("NFR-SEC-3: validOwnerRepo(%q, %q) = %v, want %v", tt.owner, tt.repo, got, tt.want)
		}
	}
}

// A-10 probe: the cookie Go sets must carry the attributes the real session cookie will use.
func TestProbeCookie(t *testing.T) {
	tests := []struct {
		name       string
		appURL     string
		sendCookie bool
		wantSecure bool
		wantHad    string
	}{
		{"first visit over https sets a Secure cookie", "https://officehours.example", false, true, `"had":false`},
		{"local http leaves Secure off so Safari keeps it", "http://localhost:5173", false, false, `"had":false`},
		{"second visit reports the cookie came back", "https://officehours.example", true, true, `"had":true`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/probe/cookie", nil)
			if tt.sendCookie {
				req.AddCookie(&http.Cookie{Name: probeCookie, Value: "1"})
			}
			rec := httptest.NewRecorder()
			newMux(config{AppURL: tt.appURL}, deps{}).ServeHTTP(rec, req)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), tt.wantHad) {
				t.Fatalf("status %d body %q, want 200 containing %q", rec.Code, rec.Body.String(), tt.wantHad)
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}
			c := cookies[0]
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Secure != tt.wantSecure {
				t.Fatalf("NFR-SEC-5: cookie attrs HttpOnly=%v SameSite=%v Path=%q Secure=%v, want true Lax \"/\" %v", c.HttpOnly, c.SameSite, c.Path, c.Secure, tt.wantSecure)
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("NEXT_PUBLIC_NEXCONN_APP_KEY", "pk_test")
	t.Setenv("NEXT_PUBLIC_NEXCONN_AREA", "sg")
	t.Setenv("APP_URL", "http://localhost:5173")
	t.Setenv("SPIKE", "1")
	want := config{AppKey: "pk_test", Area: "sg", AppURL: "http://localhost:5173", Spike: true}
	if got := configFromEnv(); got != want {
		t.Fatalf("configFromEnv() = %+v, want %+v", got, want)
	}
}

// The P0 test endpoints hand out working tokens, so SPIKE=1 must never open them on a public origin.
func TestSpikeNeedsALocalAppURL(t *testing.T) {
	tests := []struct {
		appURL, spike string
		want          bool
	}{
		{"http://localhost:5173", "1", true},
		{"http://127.0.0.1:8080", "1", true},
		{"http://[::1]:5173", "1", true},
		{"http://localhost:5173", "", false},
		{"http://localhost:5173", "true", false},
		{"https://officehours.example", "1", false},
		{"http://officehours.example", "1", false},
		{"https://localhost.officehours.example", "1", false},
		{"https://officehours.example/?next=http://localhost", "1", false},
		{"", "1", false},
		{"://not a url", "1", false},
	}
	for _, tt := range tests {
		t.Run(tt.appURL+" SPIKE="+tt.spike, func(t *testing.T) {
			t.Setenv("APP_URL", tt.appURL)
			t.Setenv("SPIKE", tt.spike)
			if got := configFromEnv().Spike; got != tt.want {
				t.Fatalf("Spike = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListenAddr(t *testing.T) {
	t.Setenv("PORT", "")
	if got := listenAddr(); got != ":8080" {
		t.Fatalf("without PORT: listenAddr() = %q, want :8080", got)
	}
	t.Setenv("PORT", "3001")
	if got := listenAddr(); got != ":3001" {
		t.Fatalf("with PORT=3001: listenAddr() = %q, want :3001", got)
	}
}

func TestOpenDB(t *testing.T) {
	if db, err := openDB(context.Background(), ""); db != nil || err != nil {
		t.Fatalf("without a URL: openDB = %v, %v; want nil, nil", db, err)
	}
	// Fake credentials are joined at run time, so CI's secret scan can stay strict about real URLs.
	const fakeLogin = "u:p"
	if _, err := openDB(context.Background(), "postgres://"+fakeLogin+"@host:notaport/db"); err == nil {
		t.Fatal("with a bad URL: openDB = nil error, want one")
	}
	// Nothing listens on port 1, and that's fine: the pool connects on first use.
	db, err := openDB(context.Background(), "postgres://"+fakeLogin+"@127.0.0.1:1/db")
	if err != nil || db == nil {
		t.Fatalf("with a well formed URL: openDB = %v, %v; want a pool", db, err)
	}
	if got := dbStatus(context.Background(), db); got != "down" {
		t.Fatalf("dbStatus on an unreachable database = %q, want down", got)
	}
}

// fakeIssuer stands in for the Nexconn client.
type fakeIssuer struct {
	err     error
	gotID   string
	gotName string
}

func (f *fakeIssuer) IssueToken(_ context.Context, userID, name string) (string, error) {
	f.gotID, f.gotName = userID, name
	return "tok_" + userID, f.err
}

// The spike token endpoint is P0 only: it must stay shut unless SPIKE=1, and it may only ever
// mint tokens for the two test users.
func TestSpikeToken(t *testing.T) {
	tests := []struct {
		name       string
		spike      bool
		issuer     *fakeIssuer
		query      string
		wantStatus int
		wantBody   string
	}{
		{"off by default", false, &fakeIssuer{}, "?user=ohspikea", 404, ""},
		{"on: a test user gets a token", true, &fakeIssuer{}, "?user=ohspikea", 200, `"userId":"ohspikea","token":"tok_ohspikea"`},
		{"on: the second test user too", true, &fakeIssuer{}, "?user=ohspikeb", 200, `"token":"tok_ohspikeb"`},
		{"NFR-SEC-2: on, but never for the bot", true, &fakeIssuer{}, "?user=oh_bot", 404, ""},
		{"NFR-SEC-2: on, but never for a real user id", true, &fakeIssuer{}, "?user=gh42", 404, ""},
		{"on: no user given", true, &fakeIssuer{}, "", 404, ""},
		{"on: Nexconn refuses", true, &fakeIssuer{err: errors.New("nexconn issue token: http 401, code 1004: signature error")}, "?user=ohspikea", 502, `"ok":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newMux(config{Spike: tt.spike}, deps{tokens: tt.issuer}).ServeHTTP(rec, httptest.NewRequest("GET", "/api/spike/token"+tt.query, nil))
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q, want %d containing %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
			if strings.Contains(rec.Body.String(), "signature error") {
				t.Fatalf("NFR-OPS-2: the upstream error text leaked to the client: %q", rec.Body.String())
			}
		})
	}
	t.Run("on, but Nexconn is not configured", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newMux(config{Spike: true}, deps{}).ServeHTTP(rec, httptest.NewRequest("GET", "/api/spike/token?user=ohspikea", nil))
		if rec.Code != 503 {
			t.Fatalf("status %d, want 503", rec.Code)
		}
	})
}

func TestOpenNexconn(t *testing.T) {
	for _, k := range []string{"NEXT_PUBLIC_NEXCONN_APP_KEY", "NEXCONN_APP_SECRET", "NEXCONN_API_BASE_URL", "NEXCONN_API_BACKUP_URL"} {
		t.Setenv(k, "")
	}
	if c, err := openNexconn(); c != nil || err != nil {
		t.Fatalf("without credentials: openNexconn = %v, %v; want nil, nil", c, err)
	}
	t.Setenv("NEXT_PUBLIC_NEXCONN_APP_KEY", "pk_test")
	t.Setenv("NEXCONN_APP_SECRET", "secret_test")
	t.Setenv("NEXCONN_API_BASE_URL", "https://api.example")
	if c, err := openNexconn(); c == nil || err != nil {
		t.Fatalf("with credentials: openNexconn = %v, %v; want a client", c, err)
	}
}
