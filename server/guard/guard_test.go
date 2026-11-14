package guard

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"officehours/session"
)

const site = "https://officehours.example"

// fakeRoles stands in for the store: who joined and who maintains project "proj1".
type fakeRoles struct {
	members, maintainers map[string]bool
	err                  error
}

func (f fakeRoles) IsMember(_ context.Context, _, userID string) (bool, error) {
	return f.members[userID], f.err
}

func (f fakeRoles) IsMaintainer(_ context.Context, _, userID string) (bool, error) {
	return f.maintainers[userID], f.err
}

func newGuard(t *testing.T, roles Roles) (*Guard, *session.Codec) {
	t.Helper()
	codec, err := session.NewCodec(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	g, err := New(site, codec, roles)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g, codec
}

// signedIn adds a session cookie for the user to the request.
func signedIn(t *testing.T, codec *session.Codec, r *http.Request, u session.User) {
	t.Helper()
	value, err := codec.Seal(session.Cookie, u, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	r.AddCookie(&http.Cookie{Name: session.Cookie, Value: value})
}

func status(err error) int {
	var refusal *Error
	if errors.As(err, &refusal) {
		return refusal.Status
	}
	if err != nil {
		return -1 // an internal error, not a refusal
	}
	return 0
}

func TestNewChecksAppURL(t *testing.T) {
	for _, tt := range []struct {
		appURL     string
		wantOrigin string // "" means refused
		wantSecure bool
	}{
		{"https://officehours.example", "https://officehours.example", true},
		{"https://officehours.example/", "https://officehours.example", true},
		{"http://localhost:5173", "http://localhost:5173", false},
		{"", "", false},
		{"officehours.example", "", false},
		{"ftp://officehours.example", "", false},
		{"https://officehours.example/app", "", false},
		{"https://officehours.example?x=1", "", false},
		{"https://user@officehours.example", "", false},
	} {
		t.Run(tt.appURL, func(t *testing.T) {
			g, err := New(tt.appURL, nil, nil)
			if tt.wantOrigin == "" {
				if err == nil {
					t.Fatalf("New(%q) accepted it", tt.appURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("New(%q): %v", tt.appURL, err)
			}
			if g.Origin() != tt.wantOrigin || g.Secure() != tt.wantSecure {
				t.Fatalf("origin %q secure %v, want %q %v", g.Origin(), g.Secure(), tt.wantOrigin, tt.wantSecure)
			}
		})
	}
}

func TestMutation(t *testing.T) {
	type body struct {
		Title string `json:"title"`
	}
	mia := session.User{ID: "gh1", Login: "mia-k"}

	tests := []struct {
		name        string
		origin      string // "-" leaves the header out
		contentType string
		payload     string
		user        *session.User
		wrongCookie bool // a value sealed for another cookie, sent as the session
		want        int  // 0 = accepted
	}{
		{name: "NFR-SEC-5: our origin, JSON, signed in", origin: site, contentType: "application/json", payload: `{"title":"Thursday"}`, user: &mia},
		{name: "NFR-SEC-5: a charset on the content type is fine", origin: site, contentType: "application/json; charset=utf-8", payload: `{"title":"Thursday"}`, user: &mia},
		{name: "NFR-SEC-5: another site's origin", origin: "https://evil.example", contentType: "application/json", payload: `{}`, user: &mia, want: 403},
		{name: "NFR-SEC-5: our host on http", origin: "http://officehours.example", contentType: "application/json", payload: `{}`, user: &mia, want: 403},
		{name: "NFR-SEC-5: our origin as a prefix", origin: site + ".evil.example", contentType: "application/json", payload: `{}`, user: &mia, want: 403},
		{name: "NFR-SEC-5: no Origin header", origin: "-", contentType: "application/json", payload: `{}`, user: &mia, want: 403},
		{name: "NFR-SEC-5: Origin null", origin: "null", contentType: "application/json", payload: `{}`, user: &mia, want: 403},
		{name: "NFR-SEC-2: signed out", origin: site, contentType: "application/json", payload: `{}`, want: 401},
		{name: "NFR-SEC-5: a login-state cookie sent as the session", origin: site, contentType: "application/json", payload: `{}`, wrongCookie: true, want: 401},
		{name: "NFR-SEC-2: a session without a user id", origin: site, contentType: "application/json", payload: `{}`, user: &session.User{Login: "nobody"}, want: 401},
		{name: "NFR-SEC-5: a form post", origin: site, contentType: "application/x-www-form-urlencoded", payload: `title=x`, user: &mia, want: 415},
		{name: "NFR-SEC-5: text/plain carrying JSON", origin: site, contentType: "text/plain", payload: `{"title":"x"}`, user: &mia, want: 415},
		{name: "NFR-SEC-5: no content type", origin: site, payload: `{"title":"x"}`, user: &mia, want: 415},
		{name: "NFR-SEC-3: not JSON", origin: site, contentType: "application/json", payload: `title=x`, user: &mia, want: 400},
		{name: "NFR-SEC-3: an unknown field", origin: site, contentType: "application/json", payload: `{"title":"x","role":"maintainer"}`, user: &mia, want: 400},
		{name: "NFR-SEC-3: two JSON values", origin: site, contentType: "application/json", payload: `{"title":"x"}{"title":"y"}`, user: &mia, want: 400},
		{name: "NFR-SEC-3: an empty body", origin: site, contentType: "application/json", payload: ``, user: &mia, want: 400},
		{name: "NFR-SEC-5: a body over 16 KB", origin: site, contentType: "application/json", payload: `{"title":"` + strings.Repeat("x", MaxBody) + `"}`, user: &mia, want: 413},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, codec := newGuard(t, fakeRoles{})
			r := httptest.NewRequest("POST", "/api/projects", strings.NewReader(tt.payload))
			if tt.origin != "-" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}
			if tt.user != nil {
				signedIn(t, codec, r, *tt.user)
			}
			if tt.wrongCookie {
				value, err := codec.Seal("oh_oauth", mia, time.Hour)
				if err != nil {
					t.Fatalf("Seal: %v", err)
				}
				r.AddCookie(&http.Cookie{Name: session.Cookie, Value: value})
			}
			var got body
			u, err := g.Mutation(httptest.NewRecorder(), r, &got)
			if status(err) != tt.want {
				t.Fatalf("Mutation: status %d (%v), want %d", status(err), err, tt.want)
			}
			if tt.want == 0 && (u.ID != "gh1" || got.Title != "Thursday") {
				t.Fatalf("accepted, but user = %+v and body = %+v", u, got)
			}
		})
	}
}

func TestMutationWithNothingToSayStillNeedsJSON(t *testing.T) {
	g, codec := newGuard(t, fakeRoles{})
	for payload, want := range map[string]int{`{}`: 0, `{"userId":"gh2"}`: 400, ``: 400} {
		r := httptest.NewRequest("POST", "/auth/logout", strings.NewReader(payload))
		r.Header.Set("Origin", site)
		r.Header.Set("Content-Type", "application/json")
		signedIn(t, codec, r, session.User{ID: "gh1"})
		if _, err := g.Mutation(httptest.NewRecorder(), r, nil); status(err) != want {
			t.Errorf("body %q: status %d, want %d", payload, status(err), want)
		}
	}
}

func TestRoles(t *testing.T) {
	roles := fakeRoles{
		members:     map[string]bool{"gh1": true, "gh2": true},
		maintainers: map[string]bool{"gh1": true},
	}
	g, _ := newGuard(t, roles)
	ctx := context.Background()
	for _, tt := range []struct {
		name string
		got  error
		want int
	}{
		{"NFR-SEC-2: a maintainer may do maintainer things", g.Maintainer(ctx, session.User{ID: "gh1"}, "proj1"), 0},
		{"NFR-SEC-2: a member calling a maintainer action", g.Maintainer(ctx, session.User{ID: "gh2"}, "proj1"), 403},
		{"NFR-SEC-2: a member may do member things", g.Member(ctx, session.User{ID: "gh2"}, "proj1"), 0},
		{"NFR-SEC-2: someone who never joined", g.Member(ctx, session.User{ID: "gh3"}, "proj1"), 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if status(tt.got) != tt.want {
				t.Fatalf("status %d (%v), want %d", status(tt.got), tt.got, tt.want)
			}
		})
	}

	// When the database can't answer, nobody is let through and the caller sees no refusal text:
	// the handler turns this into a 500.
	broken, _ := newGuard(t, fakeRoles{maintainers: map[string]bool{"gh1": true}, err: errors.New("connection reset")})
	err := broken.Maintainer(ctx, session.User{ID: "gh1"}, "proj1")
	if err == nil || status(err) != -1 {
		t.Fatalf("with a failing store: %v (status %d), want an internal error", err, status(err))
	}
}

func TestRolesWithoutAStoreRefuseInsteadOfCrashing(t *testing.T) {
	// The server starts without a database (sign-in is then off). A role check must still answer:
	// with an internal error, never with a panic and never by letting someone through.
	g, _ := newGuard(t, nil)
	for name, check := range map[string]func(context.Context, session.User, string) error{"Member": g.Member, "Maintainer": g.Maintainer} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s without a role store panicked: %v", name, r)
				}
			}()
			if err := check(context.Background(), session.User{ID: "gh1"}, "proj1"); err == nil || status(err) != -1 {
				t.Errorf("%s without a role store: %v (status %d), want an internal error", name, err, status(err))
			}
		}()
	}
}

func TestAgentActsAsItsPerson(t *testing.T) {
	g, codec := newGuard(t, nil)
	mia := session.User{ID: "gh1", Login: "mia", GitHubToken: "tok"}

	// FR-AGENT-4: a request the MCP endpoint makes for an agent carries its person in the
	// context, and every handler's User call answers that person, cookie or not.
	r := httptest.NewRequest("POST", "/api/hands/7/withdraw", nil).WithContext(WithAgent(context.Background(), mia))
	if u, err := g.User(r); err != nil || u != mia {
		t.Fatalf("User = %+v, %v; want the agent's person", u, err)
	}

	// The context wins over a cookie, so an agent never acts as whoever's browser is open.
	raj, err := codec.Seal(session.Cookie, session.User{ID: "gh2"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(&http.Cookie{Name: session.Cookie, Value: raj})
	if u, _ := g.User(r); u.ID != "gh1" {
		t.Fatalf("User = %s, want the agent's person gh1", u.ID)
	}

	// An empty person in the context is nobody.
	r = httptest.NewRequest("GET", "/api/me", nil).WithContext(WithAgent(context.Background(), session.User{}))
	if _, err := g.User(r); err == nil {
		t.Fatal("an empty agent person passed as signed in")
	}
}
