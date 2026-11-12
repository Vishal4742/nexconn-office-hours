// Package session seals small values into cookies that only this server can read or forge
// (NFR-SEC-5). The signed-in user and their GitHub token live in such a cookie and nowhere else
// (FR-AUTH-1).
//
// ponytail: sessions are stateless. Signing out deletes the browser's copy only, so a copied
// cookie works until it expires, and the only way to end every session is a new AUTH_SECRET. Move
// to a sessions table when one session must be ended from the server.
package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Cookie is the name of the session cookie.
const Cookie = "oh_session"

// Lifetime is how long a sign-in lasts.
const Lifetime = 30 * 24 * time.Hour

// User is who the session cookie says is signed in.
type User struct {
	ID          string `json:"id"` // gh<github id>; also the Nexconn user id
	Login       string `json:"login"`
	Name        string `json:"name"` // display name: GitHub's name, or the login when that is empty
	AvatarURL   string `json:"avatar"`
	GitHubToken string `json:"tok"` // never leaves the server
}

// Codec seals and opens values with AES-256-GCM.
type Codec struct {
	aead cipher.AEAD
	now  func() time.Time
}

// errInvalid is the one answer for every value Open refuses: callers must not learn why.
var errInvalid = errors.New("session: not a valid sealed value")

// NewCodec takes AUTH_SECRET: 32 random bytes in standard base64 (`openssl rand -base64 32`).
func NewCodec(secret string) (*Codec, error) {
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(key) != 32 {
		return nil, errors.New("session: AUTH_SECRET must be 32 random bytes in base64 (openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	return &Codec{aead: aead, now: time.Now}, nil
}

// sealed is what gets encrypted: the value and the moment it stops being valid.
type sealed struct {
	Exp int64           `json:"exp"` // unix seconds
	V   json.RawMessage `json:"v"`
}

// Seal encrypts v for the cookie called name. The name is bound in, so a value sealed for one
// cookie is refused as another.
func (c *Codec) Seal(name string, v any, ttl time.Duration) (string, error) {
	inner, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("session: %w", err)
	}
	plain, err := json.Marshal(sealed{Exp: c.now().Add(ttl).Unix(), V: inner})
	if err != nil {
		return "", fmt.Errorf("session: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("session: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(c.aead.Seal(nonce, nonce, plain, []byte(name))), nil
}

// Open decrypts a value sealed for the cookie called name into v. Anything this server did not
// seal for that name, and anything past its end, is refused.
func (c *Codec) Open(name, value string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return errInvalid
	}
	nonce, box := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, box, []byte(name))
	if err != nil {
		return errInvalid
	}
	var s sealed
	if err := json.Unmarshal(plain, &s); err != nil || !c.now().Before(time.Unix(s.Exp, 0)) {
		return errInvalid
	}
	if err := json.Unmarshal(s.V, v); err != nil {
		return errInvalid
	}
	return nil
}

// Write seals v and sets it as the cookie called name. secure follows APP_URL's scheme: Safari
// drops Secure cookies on http://localhost.
func (c *Codec) Write(w http.ResponseWriter, name string, v any, ttl time.Duration, secure bool) error {
	value, err := c.Seal(name, v, ttl)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(ttl / time.Second),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Read opens the cookie called name from the request into v.
func (c *Codec) Read(r *http.Request, name string, v any) error {
	cookie, err := r.Cookie(name)
	if err != nil {
		return errInvalid
	}
	return c.Open(name, cookie.Value, v)
}

// Clear tells the browser to drop the cookie called name (FR-AUTH-4).
func Clear(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}
