package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"officehours/guard"
	"officehours/session"
)

// tokenApp is the server with a fake Nexconn behind it and one signed-in user's cookie.
func tokenApp(t *testing.T, issuer tokenIssuer) (*http.ServeMux, *http.Cookie) {
	t.Helper()
	codec, err := session.NewCodec(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	g, err := guard.New(testSite, codec, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	value, err := codec.Seal(session.Cookie, session.User{ID: "gh42", Login: "mia-k", Name: "Mia K"}, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return newMux(config{AppURL: testSite}, deps{guard: g, sessions: codec, tokens: issuer}), &http.Cookie{Name: session.Cookie, Value: value}
}

func TestNexconnToken(t *testing.T) {
	post := func(mux *http.ServeMux, target, body string, cookie *http.Cookie, origin string) (*http.Response, string) {
		r := httptest.NewRequest("POST", target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		text, _ := io.ReadAll(rec.Result().Body)
		return rec.Result(), string(text)
	}

	t.Run("FR-AUTH-2: a signed-in user gets a token for their own Nexconn id", func(t *testing.T) {
		issuer := &fakeIssuer{}
		mux, cookie := tokenApp(t, issuer)
		res, body := post(mux, "/api/nexconn/token", `{}`, cookie, testSite)
		if res.StatusCode != 200 || !strings.Contains(body, `"userId":"gh42"`) || !strings.Contains(body, `"token":"tok_gh42"`) {
			t.Fatalf("status %d body %s", res.StatusCode, body)
		}
		// The name Nexconn shows in push notifications is the display name from the session.
		if issuer.gotID != "gh42" || issuer.gotName != "Mia K" {
			t.Fatalf("issued for %q (%q), want gh42 (Mia K)", issuer.gotID, issuer.gotName)
		}
		if res.Header.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("NFR-SEC-1: a token reply must not be cached: Cache-Control = %q", res.Header.Get("Cache-Control"))
		}
	})

	t.Run("NFR-SEC-2: the token is always the caller's, whatever the request says", func(t *testing.T) {
		issuer := &fakeIssuer{}
		mux, cookie := tokenApp(t, issuer)
		// The Phase 0 test endpoint took ?user=. This one must not.
		res, body := post(mux, "/api/nexconn/token?user=oh_bot&userId=gh1", `{}`, cookie, testSite)
		if res.StatusCode != 200 || issuer.gotID != "gh42" || strings.Contains(body, "oh_bot") || strings.Contains(body, "gh1\"") {
			t.Fatalf("query parameters changed the token: status %d, issued for %q, body %s", res.StatusCode, issuer.gotID, body)
		}
		issuer.gotID = ""
		res, _ = post(mux, "/api/nexconn/token", `{"userId":"oh_bot"}`, cookie, testSite)
		if res.StatusCode != 400 || issuer.gotID != "" {
			t.Fatalf("a userId in the body: status %d, issued for %q; want 400 and no token", res.StatusCode, issuer.gotID)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			cookie bool
			origin string
			want   int
		}{
			{"NFR-SEC-2: signed out", false, testSite, 401},
			{"NFR-SEC-5: another site's origin", true, "https://evil.example", 403},
			{"NFR-SEC-5: no Origin", true, "", 403},
		} {
			issuer := &fakeIssuer{}
			mux, cookie := tokenApp(t, issuer)
			if !tt.cookie {
				cookie = nil
			}
			res, body := post(mux, "/api/nexconn/token", `{}`, cookie, tt.origin)
			if res.StatusCode != tt.want || issuer.gotID != "" || strings.Contains(body, "tok_") {
				t.Errorf("%s: status %d, issued for %q, body %s; want %d and no token", tt.name, res.StatusCode, issuer.gotID, body, tt.want)
			}
		}
	})

	t.Run("NFR-OPS-2: Nexconn failing is a 502 without its error text", func(t *testing.T) {
		mux, cookie := tokenApp(t, &fakeIssuer{err: errors.New("nexconn issue token: http 401, code 1004: signature error")})
		res, body := post(mux, "/api/nexconn/token", `{}`, cookie, testSite)
		if res.StatusCode != 502 || strings.Contains(body, "signature error") || strings.Contains(body, "tok_") {
			t.Fatalf("status %d body %s", res.StatusCode, body)
		}
	})

	t.Run("no Nexconn credentials on the server is a 503", func(t *testing.T) {
		mux, cookie := tokenApp(t, nil)
		if res, _ := post(mux, "/api/nexconn/token", `{}`, cookie, testSite); res.StatusCode != 503 {
			t.Fatalf("status %d, want 503", res.StatusCode)
		}
	})

	t.Run("it is a POST: a link or an image tag can't fetch a token", func(t *testing.T) {
		mux, cookie := tokenApp(t, &fakeIssuer{})
		r := httptest.NewRequest("GET", "/api/nexconn/token", nil)
		r.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET: status %d, want 405", rec.Code)
		}
	})
}
