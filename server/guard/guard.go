// Package guard is the gate in front of the API: a request must come from our own pages, carry
// JSON, belong to a signed-in user and, where it matters, to someone with a role in the project
// (NFR-SEC-2, NFR-SEC-5). Roles come from the database, never from the request.
package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"

	"officehours/session"
)

// MaxBody is the largest request body a mutation may send. The longest field in the spec is
// 2,000 characters, so 16 KB leaves room and still stops a flood.
const MaxBody = 16 << 10

// Error is a refusal the caller may be shown as it is.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

// Roles says who may do what in a project. The store answers from the members and maintainers
// tables.
type Roles interface {
	IsMember(ctx context.Context, projectID, userID string) (bool, error)
	IsMaintainer(ctx context.Context, projectID, userID string) (bool, error)
}

// Guard checks requests for one deployment.
type Guard struct {
	origin   string // APP_URL without a path, e.g. https://officehours.example
	sessions *session.Codec
	roles    Roles
}

// New refuses an APP_URL it can't compare requests against: the Origin check and the OAuth
// redirect are exact string matches on it.
func New(appURL string, sessions *session.Codec, roles Roles) (*Guard, error) {
	u, err := url.Parse(appURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("guard: APP_URL must be the site's origin, like https://officehours.example")
	}
	return &Guard{origin: u.Scheme + "://" + u.Host, sessions: sessions, roles: roles}, nil
}

// Origin is the site's own origin: scheme, host and port.
func (g *Guard) Origin() string { return g.origin }

// Secure reports whether cookies must carry the Secure flag. It follows APP_URL's scheme, because
// Safari drops Secure cookies on http://localhost.
func (g *Guard) Secure() bool { return len(g.origin) >= 8 && g.origin[:8] == "https://" }

// User returns the signed-in user, or a 401.
func (g *Guard) User(r *http.Request) (session.User, error) {
	var u session.User
	if err := g.sessions.Read(r, session.Cookie, &u); err != nil || u.ID == "" {
		return session.User{}, &Error{http.StatusUnauthorized, "Sign in first."}
	}
	return u, nil
}

// SameOrigin refuses a request that another site made the browser send. Browsers put Origin on
// every request that isn't a plain GET, so a missing one is refused too.
func (g *Guard) SameOrigin(r *http.Request) error {
	if r.Header.Get("Origin") != g.origin {
		return &Error{http.StatusForbidden, "This request did not come from Office Hours."}
	}
	return nil
}

// Body decodes a JSON request body into dst. A nil dst means the request has nothing to say: it
// must still be JSON ({}), so every mutation looks the same to the browser's cross-site rules.
func (g *Guard) Body(w http.ResponseWriter, r *http.Request, dst any) error {
	if kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || kind != "application/json" {
		return &Error{http.StatusUnsupportedMediaType, "Send the request as JSON."}
	}
	if dst == nil {
		dst = &struct{}{}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		// One JSON value and nothing after it.
		if err = dec.Decode(&struct{}{}); err == io.EOF {
			return nil
		}
		if err == nil {
			err = errors.New("more than one JSON value")
		}
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return &Error{http.StatusRequestEntityTooLarge, "The request is too large."}
	}
	return &Error{http.StatusBadRequest, "The request is not the JSON this endpoint expects."}
}

// Mutation is the full check for a request that changes something: same origin, signed in, JSON.
func (g *Guard) Mutation(w http.ResponseWriter, r *http.Request, dst any) (session.User, error) {
	if err := g.SameOrigin(r); err != nil {
		return session.User{}, err
	}
	u, err := g.User(r)
	if err != nil {
		return session.User{}, err
	}
	if err := g.Body(w, r, dst); err != nil {
		return session.User{}, err
	}
	return u, nil
}

// Member refuses a user who has not joined the project.
func (g *Guard) Member(ctx context.Context, u session.User, projectID string) error {
	return g.role(ctx, g.roles.IsMember, u, projectID, "Join this room first.")
}

// Maintainer refuses a user who does not maintain the project.
func (g *Guard) Maintainer(ctx context.Context, u session.User, projectID string) error {
	return g.role(ctx, g.roles.IsMaintainer, u, projectID, "Only a maintainer of this project can do that.")
}

func (g *Guard) role(ctx context.Context, has func(context.Context, string, string) (bool, error), u session.User, projectID, refusal string) error {
	ok, err := has(ctx, projectID, u.ID)
	if err != nil {
		return fmt.Errorf("guard: role lookup: %w", err)
	}
	if !ok {
		return &Error{http.StatusForbidden, refusal}
	}
	return nil
}
