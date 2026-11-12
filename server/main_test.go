package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("NEXT_PUBLIC_NEXCONN_APP_KEY", "pk_test")
	t.Setenv("NEXT_PUBLIC_NEXCONN_AREA", "sg")
	t.Setenv("APP_URL", "http://localhost:5173")
	want := config{AppKey: "pk_test", Area: "sg", AppURL: "http://localhost:5173"}
	if got := configFromEnv(); got != want {
		t.Fatalf("configFromEnv() = %+v, want %+v", got, want)
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
	stuck   chan struct{} // when set, IssueToken never answers; closed once its context is cancelled
}

func (f *fakeIssuer) IssueToken(ctx context.Context, userID, name string) (string, error) {
	if f.stuck != nil {
		<-ctx.Done()
		close(f.stuck)
		return "", ctx.Err()
	}
	f.gotID, f.gotName = userID, name
	return "tok_" + userID, f.err
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

func TestEveryRequestHasADeadline(t *testing.T) {
	// A database or a chat service that never answers must not hold a request open for as long
	// as the platform allows: the request is given up on, and what it was waiting for is told so.
	issuer := &fakeIssuer{stuck: make(chan struct{})}
	mux, cookie := tokenApp(t, issuer)
	handler := withDeadline(mux, 50*time.Millisecond)

	r := httptest.NewRequest("POST", "/api/nexconn/token", strings.NewReader(`{}`))
	r.Header.Set("Origin", testSite)
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"ok":false`) || !strings.Contains(rec.Body.String(), "too long") {
		t.Fatalf("status %d body %q, want 503 with our own sentence", rec.Code, rec.Body.String())
	}
	select {
	case <-issuer.stuck:
	case <-time.After(2 * time.Second):
		t.Fatal("the call the request was waiting on was never cancelled")
	}

	// A request that answers in time is passed through untouched.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("a fast request: status %d, type %q, body %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
