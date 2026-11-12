package session

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testKey is 32 bytes of nothing secret, in the format of AUTH_SECRET (`openssl rand -base64 32`).
var testKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func newCodec(t *testing.T, key string) *Codec {
	t.Helper()
	c, err := NewCodec(key)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

func TestNewCodecNeedsA32ByteKey(t *testing.T) {
	for name, key := range map[string]string{
		"empty":       "",
		"not base64":  "!!!not-base64!!!",
		"16 bytes":    base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")),
		"33 bytes":    base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdefX")),
		"hex, not 64": "0123456789abcdef",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewCodec(key); err == nil {
				t.Fatal("NewCodec accepted a key that is not 32 bytes of base64")
			}
		})
	}
}

func TestSealedValuesRoundTrip(t *testing.T) {
	c := newCodec(t, testKey)
	in := User{ID: "gh42", Login: "mia-k", Name: "Mia K", AvatarURL: "https://avatars.githubusercontent.com/u/42", GitHubToken: "gho_test_token"}
	sealed, err := c.Seal(Cookie, in, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// NFR-SEC-5 / FR-AUTH-1: the cookie is encrypted, so the GitHub token can't be read out of it.
	if strings.Contains(sealed, "gho_test_token") || strings.Contains(sealed, "mia-k") {
		t.Fatalf("the sealed value shows its contents: %s", sealed)
	}
	var out User
	if err := c.Open(Cookie, sealed, &out); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if out != in {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
	// Two seals of the same value differ: each gets a fresh nonce.
	again, _ := c.Seal(Cookie, in, time.Hour)
	if again == sealed {
		t.Fatal("two seals of the same value are identical")
	}
}

func TestOpenRefusesWhatItDidNotSeal(t *testing.T) {
	c := newCodec(t, testKey)
	sealed, err := c.Seal(Cookie, User{ID: "gh42"}, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	other := newCodec(t, base64.StdEncoding.EncodeToString([]byte("another-key-another-key-another!")))

	flipped := []byte(sealed)
	flipped[len(flipped)/2] ^= 0x01 // may leave base64, which must be refused just the same

	tests := map[string]func() error{
		"NFR-SEC-5: one changed character":     func() error { return c.Open(Cookie, string(flipped), new(User)) },
		"NFR-SEC-5: sealed with another key":   func() error { return other.Open(Cookie, sealed, new(User)) },
		"NFR-SEC-5: sealed for another cookie": func() error { return c.Open("oh_oauth", sealed, new(User)) },
		"NFR-SEC-5: cut short":                 func() error { return c.Open(Cookie, sealed[:10], new(User)) },
		"NFR-SEC-5: empty":                     func() error { return c.Open(Cookie, "", new(User)) },
		"NFR-SEC-5: not base64":                func() error { return c.Open(Cookie, "%%%", new(User)) },
		"NFR-SEC-5: plain JSON, never encrypted": func() error {
			return c.Open(Cookie, base64.RawURLEncoding.EncodeToString([]byte(`{"id":"gh1"}`)), new(User))
		},
	}
	for name, open := range tests {
		t.Run(name, func(t *testing.T) {
			if err := open(); err == nil {
				t.Fatal("Open accepted it")
			}
		})
	}
}

func TestOpenRefusesAnExpiredValue(t *testing.T) {
	c := newCodec(t, testKey)
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	sealed, err := c.Seal(Cookie, User{ID: "gh42"}, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	now = now.Add(59 * time.Minute)
	if err := c.Open(Cookie, sealed, new(User)); err != nil {
		t.Fatalf("Open one minute before the end: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := c.Open(Cookie, sealed, new(User)); err == nil {
		t.Fatal("NFR-SEC-5: Open accepted a value one minute past its end")
	}
}

func TestCookieAttributes(t *testing.T) {
	c := newCodec(t, testKey)
	for _, tt := range []struct {
		name       string
		secure     bool
		wantSecure bool
	}{
		{"NFR-SEC-5: https site", true, true},
		{"NFR-SEC-5: http://localhost (Safari drops Secure cookies there)", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := c.Write(rec, Cookie, User{ID: "gh42"}, 30*24*time.Hour, tt.secure); err != nil {
				t.Fatalf("Write: %v", err)
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}
			got := cookies[0]
			if got.Name != Cookie || !got.HttpOnly || got.SameSite != http.SameSiteLaxMode || got.Path != "/" {
				t.Errorf("cookie = %+v, want %s with HttpOnly, SameSite=Lax and Path=/", got, Cookie)
			}
			if got.Secure != tt.wantSecure {
				t.Errorf("Secure = %v, want %v", got.Secure, tt.wantSecure)
			}
			if got.MaxAge != 30*24*3600 {
				t.Errorf("MaxAge = %d, want 30 days", got.MaxAge)
			}

			// The browser sends it back, and Read gets the user out again.
			req := httptest.NewRequest("GET", "/api/me", nil)
			req.AddCookie(got)
			var u User
			if err := c.Read(req, Cookie, &u); err != nil || u.ID != "gh42" {
				t.Fatalf("Read = %+v, %v, want gh42", u, err)
			}
		})
	}
}

func TestReadWithoutACookieIsAnError(t *testing.T) {
	c := newCodec(t, testKey)
	if err := c.Read(httptest.NewRequest("GET", "/api/me", nil), Cookie, new(User)); err == nil {
		t.Fatal("Read without a cookie = nil error")
	}
}

func TestClearExpiresTheCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	Clear(rec, Cookie, true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != Cookie || cookies[0].MaxAge >= 0 || cookies[0].Value != "" {
		t.Fatalf("FR-AUTH-4: Clear set %+v, want an empty, expired %s", cookies, Cookie)
	}
	if !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].Path != "/" {
		t.Errorf("Clear must use the same attributes the cookie was set with, got %+v", cookies[0])
	}
}
