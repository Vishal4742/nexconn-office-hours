package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"officehours/guard"
	"officehours/session"
	"officehours/store"
)

// oauthCookie carries one sign-in attempt from /auth/github/login to the callback: sealed, HttpOnly
// and gone after ten minutes or one use.
const (
	oauthCookie = "oh_oauth"
	oauthTTL    = 10 * time.Minute
)

// oauthState is what that cookie holds.
type oauthState struct {
	State    string `json:"s"` // must come back in the callback's query (CSRF)
	Verifier string `json:"v"` // PKCE: only its hash went to GitHub
	Next     string `json:"n"` // where to land after signing in
}

// userStore is the part of the store that sign-in needs.
type userStore interface {
	UpsertUser(ctx context.Context, u store.User) error
}

// safeNext keeps only the two places a sign-in may land: the dashboard, or a project's page
// (FR-PROJ-3). Anything else, including every absolute URL, becomes the dashboard.
func safeNext(next string) string {
	parts := strings.Split(next, "/")
	if len(parts) == 4 && parts[0] == "" && parts[1] == "p" && validOwnerRepo(parts[2], parts[3]) {
		return next
	}
	return "/dashboard"
}

// signInFailed sends the browser back to the landing page, which shows one sentence per reason.
// A top-level navigation must never end on a JSON error.
func signInFailed(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?signin="+reason, http.StatusFound)
}

// login starts a sign-in (FR-AUTH-1): a fresh state and PKCE verifier go into the sealed cookie,
// and the browser goes to GitHub.
func (d deps) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	origin, err := url.Parse(d.guard.Origin())
	if err != nil {
		signInFailed(w, r, "server")
		return
	}
	// A deployment answers on more than one host. The state cookie must be set on the host
	// GitHub sends the browser back to, so a login that starts elsewhere is sent there first.
	// Behind a proxy the browser's host is in X-Forwarded-Host; if it can't be seen at all the
	// redirect happens once (the "hop" mark) and never loops. The header only decides whether
	// to redirect to our own fixed origin, so a forged one gains nothing.
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	q := r.URL.Query()
	if host != origin.Host && q.Get("hop") == "" {
		q.Set("hop", "1")
		http.Redirect(w, r, d.guard.Origin()+r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return
	}
	if d.github == nil || d.users == nil {
		signInFailed(w, r, "setup")
		return
	}
	st := oauthState{State: rand.Text(), Verifier: rand.Text() + rand.Text(), Next: safeNext(q.Get("next"))}
	if err := d.sessions.Write(w, oauthCookie, st, oauthTTL, d.guard.Secure()); err != nil {
		log.Printf("sign-in: seal state: %v", err)
		signInFailed(w, r, "server")
		return
	}
	http.Redirect(w, r, d.github.AuthorizeURL(st.State, st.Verifier), http.StatusFound)
}

// callback finishes a sign-in: GitHub sent the browser back with a code, which is traded for the
// user's token. The token stays inside the sealed session cookie (FR-AUTH-1).
func (d deps) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if d.github == nil || d.users == nil {
		signInFailed(w, r, "setup")
		return
	}
	var st oauthState
	err := d.sessions.Read(r, oauthCookie, &st)
	session.Clear(w, oauthCookie, d.guard.Secure()) // one attempt, one use
	q := r.URL.Query()
	if err != nil || st.State == "" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		signInFailed(w, r, "expired")
		return
	}
	if q.Get("error") != "" { // access_denied: the user pressed Cancel
		signInFailed(w, r, "cancelled")
		return
	}
	code := q.Get("code")
	if code == "" {
		signInFailed(w, r, "github")
		return
	}

	token, err := d.github.Exchange(r.Context(), code, st.Verifier)
	if err != nil {
		log.Printf("sign-in: %v", err)
		signInFailed(w, r, "github")
		return
	}
	profile, err := d.github.User(r.Context(), token)
	if err != nil {
		log.Printf("sign-in: %v", err)
		signInFailed(w, r, "github")
		return
	}
	u := session.User{
		ID:    "gh" + strconv.FormatInt(profile.ID, 10),
		Login: profile.Login, Name: profile.DisplayName(), AvatarURL: profile.AvatarURL,
		GitHubToken: token,
	}
	if err := d.users.UpsertUser(r.Context(), store.User{ID: u.ID, GitHubID: profile.ID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL}); err != nil {
		log.Printf("sign-in: %v", err)
		signInFailed(w, r, "server")
		return
	}
	if err := d.sessions.Write(w, session.Cookie, u, session.Lifetime, d.guard.Secure()); err != nil {
		log.Printf("sign-in: seal session: %v", err)
		signInFailed(w, r, "server")
		return
	}
	http.Redirect(w, r, safeNext(st.Next), http.StatusFound)
}

// logout clears the session cookie (FR-AUTH-4). It needs our Origin, so another site can't sign
// people out, and it works when nobody is signed in.
func (d deps) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := d.guard.SameOrigin(r); err != nil {
		writeError(w, err)
		return
	}
	if err := d.guard.Body(w, r, nil); err != nil {
		writeError(w, err)
		return
	}
	session.Clear(w, session.Cookie, d.guard.Secure())
	writeJSON(w, http.StatusOK, envelope{OK: true})
}

// me tells the page who is signed in. The GitHub token is not part of the answer.
func (d deps) me(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.User(r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: struct {
		ID        string `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatarUrl"`
	}{u.ID, u.Login, u.Name, u.AvatarURL}})
}

// writeError answers with a guard refusal as it is, and hides anything else behind a 500.
func writeError(w http.ResponseWriter, err error) {
	var refusal *guard.Error
	if errors.As(err, &refusal) {
		writeJSON(w, refusal.Status, envelope{Error: refusal.Msg})
		return
	}
	// The session is fine but its user row is gone. A new sign-in writes the row again.
	if errors.Is(err, store.ErrUnknownUser) {
		writeJSON(w, http.StatusUnauthorized, envelope{Error: "Your sign-in is out of date. Sign in again to continue."})
		return
	}
	log.Printf("internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, envelope{Error: "Something went wrong on our side. Try again."})
}
