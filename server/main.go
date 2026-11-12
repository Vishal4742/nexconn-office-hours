// Command server is the Office Hours backend: the JSON API, GitHub sign-in and the public FAQ pages
// (still placeholders until P3).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"time"

	"officehours/github"
	"officehours/guard"
	"officehours/nexconn"
	"officehours/session"
	"officehours/store"
)

// config is what the server reads from its environment. Secrets join it as their features land.
type config struct {
	AppKey string // Nexconn App Key: public by design, the browser needs it
	Area   string // Nexconn data center for the client SDK
	AppURL string // the origin browsers use: the Origin check, the cookie Secure flag and the OAuth callback follow it
}

func configFromEnv() config {
	return config{
		AppKey: os.Getenv("NEXT_PUBLIC_NEXCONN_APP_KEY"),
		Area:   os.Getenv("NEXT_PUBLIC_NEXCONN_AREA"),
		AppURL: os.Getenv("APP_URL"),
	}
}

// envelope is the one shape every API response uses.
type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

// pinger is the part of the connection pool the health check needs.
type pinger interface {
	Ping(ctx context.Context) error
}

// dbStatus is "off" without a database, "up" when it answers within 3 s and "down" otherwise.
func dbStatus(ctx context.Context, db pinger) string {
	if db == nil {
		return "off"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		log.Printf("database ping failed: %v", err)
		return "down"
	}
	return "up"
}

// tokenIssuer is the part of the Nexconn client that hands out access tokens.
type tokenIssuer interface {
	IssueToken(ctx context.Context, userID, name string) (string, error)
}

// deps are the outside systems the handlers use. A nil db, tokens, rooms, github, users or
// projects means "not configured" and the routes that need it say so. guard and sessions are
// always there: main refuses to start without APP_URL and AUTH_SECRET.
type deps struct {
	db       pinger
	tokens   tokenIssuer
	rooms    roomService
	guard    *guard.Guard
	sessions *session.Codec
	github   *github.Client
	users    userStore
	projects projectStore
}

func newMux(cfg config, d deps) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, envelope{OK: true, Data: map[string]string{"db": dbStatus(r.Context(), d.db)}})
	})

	// Sign-in (FR-AUTH-1, FR-AUTH-4).
	mux.HandleFunc("GET /auth/github/login", d.login)
	mux.HandleFunc("GET /auth/github/callback", d.callback)
	mux.HandleFunc("POST /auth/logout", d.logout)
	mux.HandleFunc("GET /api/me", d.me)
	mux.HandleFunc("POST /api/nexconn/token", d.nexconnToken) // FR-AUTH-2

	// Projects and rooms (FR-PROJ-1..4). Everything about one project is under /api/p/{owner}/{repo},
	// like its page: the project's id never has to leave the server.
	mux.HandleFunc("POST /api/projects", d.registerProject)
	mux.HandleFunc("GET /api/me/projects", d.myProjects)
	mux.HandleFunc("GET /api/p/{owner}/{repo}", d.projectPage)
	mux.HandleFunc("POST /api/p/{owner}/{repo}/join", d.joinRoom)
	mux.HandleFunc("GET /api/p/{owner}/{repo}/members", d.roomMembers)

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, _ *http.Request) {
		if cfg.AppKey == "" || cfg.Area == "" {
			writeJSON(w, http.StatusServiceUnavailable, envelope{Error: "The server has no Nexconn client config yet."})
			return
		}
		writeJSON(w, http.StatusOK, envelope{OK: true, Data: struct {
			AppKey string `json:"appKey"`
			Area   string `json:"area"`
		}{cfg.AppKey, cfg.Area}})
	})

	mux.HandleFunc("GET /p/{owner}/{repo}/faq", faqPlaceholder)
	mux.HandleFunc("GET /p/{owner}/{repo}/faq/{slug}", faqPlaceholder)
	return mux
}

// faqPlaceholder stands in for the server-rendered FAQ pages (FR-FAQ-3) until P3 replaces it with
// html/template pages. It exists now so the first deploy can prove these routes reach Go.
func faqPlaceholder(w http.ResponseWriter, r *http.Request) {
	owner, repo, slug := r.PathValue("owner"), r.PathValue("repo"), r.PathValue("slug")
	if !validOwnerRepo(owner, repo) || (slug != "" && !slugRe.MatchString(slug)) {
		http.NotFound(w, r)
		return
	}
	what := "FAQ list"
	if slug != "" {
		what = "FAQ entry: " + slug
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width, initial-scale=1">`+
		`<title>%[1]s FAQ · Office Hours</title><h1>%[1]s</h1><p>%[2]s</p>`+
		`<p>The public FAQ pages arrive in P3. This page shows the route reaches the Go server.</p>`,
		html.EscapeString(owner+"/"+repo), html.EscapeString(what))
}

// listenAddr honours $PORT (Vercel sets it) and falls back to 8080 for local dev.
func listenAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

// openDB returns nil when no database is configured, and the health check then says "off".
func openDB(ctx context.Context, url string) (*store.Store, error) {
	if url == "" {
		return nil, nil
	}
	pool, err := store.Open(ctx, url)
	if err != nil {
		return nil, err
	}
	return store.New(pool), nil
}

// openGitHub returns nil when the OAuth app's credentials are missing; sign-in then answers
// "not set up". The callback address is built from APP_URL, never from a request.
func openGitHub(origin string) (*github.Client, error) {
	id, secret := os.Getenv("AUTH_GITHUB_ID"), os.Getenv("AUTH_GITHUB_SECRET")
	if id == "" || secret == "" {
		return nil, nil
	}
	return github.New(github.Config{ClientID: id, ClientSecret: secret, RedirectURL: origin + "/auth/github/callback"})
}

// openNexconn returns nil when the server has no Nexconn credentials, e.g. in CI.
func openNexconn() (*nexconn.Client, error) {
	key, secret, base := os.Getenv("NEXT_PUBLIC_NEXCONN_APP_KEY"), os.Getenv("NEXCONN_APP_SECRET"), os.Getenv("NEXCONN_API_BASE_URL")
	if key == "" || secret == "" || base == "" {
		return nil, nil
	}
	return nexconn.New(nexconn.Config{AppKey: key, AppSecret: secret, BaseURL: base, BackupURL: os.Getenv("NEXCONN_API_BACKUP_URL")})
}

// newDeps reads the environment and connects what is configured. APP_URL and AUTH_SECRET are not
// optional: without them no request could be checked and no cookie sealed.
func newDeps(ctx context.Context, cfg config) (deps, error) {
	sessions, err := session.NewCodec(os.Getenv("AUTH_SECRET"))
	if err != nil {
		return deps{}, err
	}
	db, err := openDB(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return deps{}, fmt.Errorf("database: %w", err)
	}
	g, err := guard.New(cfg.AppURL, sessions, nil) // roles join with the first route that needs one (the room's member list)
	if err != nil {
		return deps{}, err
	}
	gh, err := openGitHub(g.Origin())
	if err != nil {
		return deps{}, err
	}
	chat, err := openNexconn()
	if err != nil {
		return deps{}, fmt.Errorf("nexconn: %w", err)
	}
	d := deps{guard: g, sessions: sessions, github: gh}
	// A nil pointer inside an interface would not compare equal to nil, so each is set only when
	// it exists.
	if chat != nil {
		d.tokens, d.rooms = chat, chat
	}
	if db != nil {
		d.db, d.users, d.projects = db, db, db
	}
	return d, nil
}

func main() {
	cfg := configFromEnv()
	d, err := newDeps(context.Background(), cfg)
	if err != nil {
		log.Fatalf("cannot start: %v", err)
	}
	if d.github == nil || d.users == nil {
		log.Print("sign-in is off: it needs AUTH_GITHUB_ID, AUTH_GITHUB_SECRET and DATABASE_URL")
	}
	srv := &http.Server{
		Addr:              listenAddr(),
		Handler:           newMux(cfg, d),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("office hours server listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
