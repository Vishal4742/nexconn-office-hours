package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"officehours/github"
	"officehours/guard"
	"officehours/session"
	"officehours/store"
)

// What GitHub says the caller may do on a repo.
const (
	adminRights    = `{"admin":true,"maintain":true,"push":true,"triage":true,"pull":true}`
	maintainRights = `{"admin":false,"maintain":true,"push":true,"triage":true,"pull":true}`
	pushRights     = `{"admin":false,"maintain":false,"push":true,"triage":true,"pull":true}`
)

// githubRepo is GitHub's reply for one public repository. An empty permissions leaves the key out.
func githubRepo(owner, ownerType, name, permissions string) string {
	reply := `{"name":"` + name + `","private":false,"owner":{"login":"` + owner + `","type":"` + ownerType + `"},"description":"A tiny HTTP cache."`
	if permissions != "" {
		reply += `,"permissions":` + permissions
	}
	return reply + `}`
}

// fakeRepoAPI is GitHub's repository endpoint: one reply whatever is asked, and a record of what was.
type fakeRepoAPI struct {
	*httptest.Server
	status int
	reply  string
	hang   chan struct{} // closed to release a GitHub that does not answer
	asked  []string      // "Authorization header + space + path"
}

func newFakeRepoAPI(t *testing.T) *fakeRepoAPI {
	t.Helper()
	f := &fakeRepoAPI{status: 200, reply: githubRepo("PaperPlane", "Organization", "Stash", maintainRights)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		f.asked = append(f.asked, r.Header.Get("Authorization")+" "+r.URL.Path)
		if f.hang != nil {
			<-f.hang
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		io.WriteString(w, f.reply)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// fakeRooms stands in for Nexconn's group calls. What happened goes into the journal it shares
// with fakeProjects, so a test can read the order across both.
type fakeRooms struct {
	events    *[]string
	names     map[string]string // group id → its name
	createErr error
	joinErr   error
}

func (f *fakeRooms) CreateGroup(_ context.Context, channelID, name, ownerID string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.names[channelID] = name
	*f.events = append(*f.events, "create group "+channelID+" owner "+ownerID)
	return nil
}

func (f *fakeRooms) JoinGroup(_ context.Context, channelID string, userIDs ...string) error {
	if f.joinErr != nil {
		return f.joinErr
	}
	*f.events = append(*f.events, "join "+channelID+" "+strings.Join(userIDs, ","))
	return nil
}

// fakeProjects is the projects, maintainers and members tables in memory, with the store's rules.
type fakeProjects struct {
	events       *[]string
	byRepo       map[string]store.Project // key: lower-case owner/repo
	maintainers  map[string]bool          // "projectID userID"
	members      map[string]bool
	beforeCreate func() // runs first in CreateProject: another request that wins the race
	err          error  // every call fails with it
	createErr    error  // CreateProject fails with it
}

func repoKey(owner, repo string) string { return strings.ToLower(owner + "/" + repo) }

func (f *fakeProjects) seenBy(p store.Project, userID string) store.Project {
	p.IsMember, p.IsMaintainer = f.members[p.ID+" "+userID], f.maintainers[p.ID+" "+userID]
	return p
}

func (f *fakeProjects) ProjectByRepo(_ context.Context, owner, repo, viewerID string) (store.Project, bool, error) {
	if f.err != nil {
		return store.Project{}, false, f.err
	}
	p, ok := f.byRepo[repoKey(owner, repo)]
	if !ok {
		return store.Project{}, false, nil
	}
	return f.seenBy(p, viewerID), true, nil
}

func (f *fakeProjects) CreateProject(_ context.Context, p store.Project, creatorID string) (store.Project, bool, error) {
	if err := cmp.Or(f.err, f.createErr); err != nil {
		return store.Project{}, false, err
	}
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	if existing, ok := f.byRepo[repoKey(p.Owner, p.Repo)]; ok {
		return f.seenBy(existing, creatorID), false, nil
	}
	f.byRepo[repoKey(p.Owner, p.Repo)] = p
	f.maintainers[p.ID+" "+creatorID], f.members[p.ID+" "+creatorID] = true, true
	*f.events = append(*f.events, "insert project "+p.ID)
	return f.seenBy(p, creatorID), true, nil
}

func (f *fakeProjects) AddMaintainer(_ context.Context, projectID, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.maintainers[projectID+" "+userID], f.members[projectID+" "+userID] = true, true
	*f.events = append(*f.events, "add maintainer "+projectID+" "+userID)
	return nil
}

func (f *fakeProjects) ProjectsOf(_ context.Context, userID string) ([]store.Project, error) {
	if f.err != nil {
		return nil, f.err
	}
	var mine []store.Project
	for _, p := range f.byRepo {
		if f.members[p.ID+" "+userID] {
			mine = append(mine, f.seenBy(p, userID))
		}
	}
	sort.Slice(mine, func(i, j int) bool {
		return repoKey(mine[i].Owner, mine[i].Repo) < repoKey(mine[j].Owner, mine[j].Repo)
	})
	return mine, nil
}

// projectsApp is the server with a fake GitHub, a fake Nexconn and in-memory tables behind it.
type projectsApp struct {
	mux      *http.ServeMux
	gh       *fakeRepoAPI
	rooms    *fakeRooms
	projects *fakeProjects
	events   *[]string // what the fakes did, in order
	codec    *session.Codec
	deps     deps
}

func newProjectsApp(t *testing.T) *projectsApp {
	t.Helper()
	codec, err := session.NewCodec(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	g, err := guard.New(testSite, codec, nil)
	if err != nil {
		t.Fatalf("guard.New: %v", err)
	}
	gh := newFakeRepoAPI(t)
	client, err := github.New(github.Config{
		ClientID: "Iv1.test-client-id", ClientSecret: testClientSecret,
		RedirectURL: testSite + "/auth/github/callback", APIURL: gh.URL,
		Timeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("github.New: %v", err)
	}
	events := &[]string{}
	a := &projectsApp{
		gh: gh, events: events, codec: codec,
		rooms:    &fakeRooms{events: events, names: map[string]string{}},
		projects: &fakeProjects{events: events, byRepo: map[string]store.Project{}, maintainers: map[string]bool{}, members: map[string]bool{}},
	}
	a.deps = deps{guard: g, sessions: codec, github: client, rooms: a.rooms, projects: a.projects}
	a.mux = newMux(config{AppURL: testSite}, a.deps)
	return a
}

// as is the session cookie of a signed-in user, with their GitHub token sealed inside.
func (a *projectsApp) as(t *testing.T, id string) *http.Cookie {
	t.Helper()
	value, err := a.codec.Seal(session.Cookie, session.User{ID: id, Login: "login-" + id, Name: "Name " + id, GitHubToken: testUserToken}, time.Hour)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return &http.Cookie{Name: session.Cookie, Value: value}
}

// call sends one request to mux the way a page of ours would: JSON, and our Origin on a POST.
func call(mux *http.ServeMux, method, target, body string, cookie *http.Cookie, mutate ...func(*http.Request)) (*http.Response, string) {
	var reader io.Reader
	if method == "POST" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, reader)
	if method == "POST" {
		r.Header.Set("Origin", testSite)
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for _, m := range mutate {
		m(r)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	text, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(text)
}

func (a *projectsApp) register(owner, repo string, cookie *http.Cookie) (*http.Response, string) {
	return call(a.mux, "POST", "/api/projects", `{"owner":"`+owner+`","repo":"`+repo+`"}`, cookie)
}

// projectReply is a project as the API describes it.
type projectReply struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Description  string `json:"description"`
	IsMember     bool   `json:"isMember"`
	IsMaintainer bool   `json:"isMaintainer"`
}

// decode reads the data of an ok envelope into out and fails the test on anything else.
func decode(t *testing.T, body string, out any) {
	t.Helper()
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil || !env.OK {
		t.Fatalf("not an ok envelope (%v): %s", err, body)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("data: %v in %s", err, body)
	}
}

// refused fails the test unless the reply is a refusal with the wanted status and words.
func refused(t *testing.T, name string, res *http.Response, body string, status int, says string) {
	t.Helper()
	if res.StatusCode != status || !strings.Contains(body, `"ok":false`) || !strings.Contains(body, says) {
		t.Errorf("%s: status %d body %s; want %d and a sentence with %q", name, res.StatusCode, body, status, says)
	}
	if strings.Contains(body, testUserToken) {
		t.Errorf("%s: NFR-SEC-1: the reply contains the user's GitHub token: %s", name, body)
	}
}

func TestRegisterProject(t *testing.T) {
	t.Run("FR-PROJ-2: the room is created first, then the project, with the registrant as its maintainer", func(t *testing.T) {
		a := newProjectsApp(t)
		res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		var p projectReply
		decode(t, body, &p)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status %d, want 201: %s", res.StatusCode, body)
		}
		// The project is called what GitHub calls the repo, not what was typed.
		if p.Owner != "PaperPlane" || p.Repo != "Stash" || p.Description != "A tiny HTTP cache." || !p.IsMember || !p.IsMaintainer {
			t.Fatalf("project = %+v", p)
		}
		// Plan decision 2: the id is random, because it names the Nexconn group.
		if !regexp.MustCompile(`^[a-z2-7]{26}$`).MatchString(p.ID) {
			t.Fatalf("id = %q, want 26 random lower-case characters", p.ID)
		}
		// Plan decision 3: a project row always means a room that exists and holds its maintainer.
		group := "p" + p.ID
		want := []string{"create group " + group + " owner oh_bot", "join " + group + " gh42", "insert project " + p.ID}
		if !slices.Equal(*a.events, want) {
			t.Fatalf("order = %q, want %q", *a.events, want)
		}
		if a.rooms.names[group] != "PaperPlane/Stash" {
			t.Fatalf("the room is named %q, want PaperPlane/Stash", a.rooms.names[group])
		}
		// FR-PROJ-1: GitHub was asked about this repo with the user's own token.
		if want := []string{"Bearer " + testUserToken + " /repos/paperplane/stash"}; !slices.Equal(a.gh.asked, want) {
			t.Fatalf("GitHub was asked %q, want %q", a.gh.asked, want)
		}
		if strings.Contains(body, testUserToken) {
			t.Fatalf("NFR-SEC-1: the reply contains the user's GitHub token: %s", body)
		}

		// A second repo gets an id of its own.
		a.gh.reply = githubRepo("mia-k", "User", "notes", adminRights)
		_, body = a.register("mia-k", "notes", a.as(t, "gh42"))
		var other projectReply
		decode(t, body, &other)
		if other.ID == p.ID || other.Owner != "mia-k" {
			t.Fatalf("second project = %+v, first id %s", other, p.ID)
		}
	})

	t.Run("FR-PROJ-2: registering it again returns the same project and creates nothing", func(t *testing.T) {
		a := newProjectsApp(t)
		mia := a.as(t, "gh42")
		var first, again projectReply
		_, body := a.register("paperplane", "stash", mia)
		decode(t, body, &first)
		*a.events = nil

		res, body := a.register("PAPERPLANE", "STASH", mia)
		decode(t, body, &again)
		if res.StatusCode != http.StatusOK || again != first {
			t.Fatalf("status %d project %+v, want 200 and %+v", res.StatusCode, again, first)
		}
		if len(*a.events) != 0 {
			t.Fatalf("the second registration did %q, want nothing", *a.events)
		}
	})

	t.Run("plan decision 4: a second maintainer of the repo joins the project that exists", func(t *testing.T) {
		a := newProjectsApp(t)
		var first, second projectReply
		_, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		decode(t, body, &first)
		*a.events = nil

		res, body := a.register("paperplane", "stash", a.as(t, "gh7"))
		decode(t, body, &second)
		if res.StatusCode != http.StatusOK || second.ID != first.ID || !second.IsMaintainer || !second.IsMember {
			t.Fatalf("status %d project %+v, want 200 and a maintainer of %s", res.StatusCode, second, first.ID)
		}
		// The room first, then the rows: the same order as a new project.
		want := []string{"join p" + first.ID + " gh7", "add maintainer " + first.ID + " gh7"}
		if !slices.Equal(*a.events, want) {
			t.Fatalf("order = %q, want %q", *a.events, want)
		}
	})

	t.Run("FR-PROJ-2: two registrations at the same moment end in one project", func(t *testing.T) {
		a := newProjectsApp(t)
		const winner = "winnerwinnerwinnerwinner22"
		// Someone else's registration lands between this request's lookup and its insert.
		a.projects.beforeCreate = func() {
			a.projects.byRepo[repoKey("PaperPlane", "Stash")] = store.Project{ID: winner, Owner: "PaperPlane", Repo: "Stash"}
		}
		res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		var p projectReply
		decode(t, body, &p)
		if res.StatusCode != http.StatusOK || p.ID != winner || !p.IsMaintainer || !p.IsMember {
			t.Fatalf("status %d project %+v, want 200 and a maintainer of the winner's project", res.StatusCode, p)
		}
		// The group this request made under its own id stays behind, unused. The caller is in the
		// winner's room, and no second project row exists.
		events := *a.events
		if len(events) != 4 || !slices.Equal(events[2:], []string{"join p" + winner + " gh42", "add maintainer " + winner + " gh42"}) {
			t.Fatalf("events = %q", events)
		}
		if strings.Contains(strings.Join(events, "\n"), "insert project") || strings.HasPrefix(events[0], "create group p"+winner) {
			t.Fatalf("events = %q: the loser must not record a project or re-create the winner's room", events)
		}
	})

	t.Run("FR-PROJ-1: who may register, and what the others are told", func(t *testing.T) {
		private := `{"name":"notes","private":true,"owner":{"login":"mia-k","type":"User"},"description":null,"permissions":` + adminRights + `}`
		for _, tt := range []struct {
			name   string
			status int // GitHub's
			reply  string
			want   int
			says   string
			never  string // words that would mislead here
		}{
			{"push rights are not enough", 200, githubRepo("mia-k", "User", "stash", pushRights), 403, "admin or maintain rights on mia-k/stash", "approve"},
			{"on an organization's repo the refusal names the other cause", 200, githubRepo("PaperPlane", "Organization", "Stash", pushRights), 403, "ask an owner of PaperPlane to approve Office Hours", ""},
			{"GitHub knows no such repo", 404, `{"message":"Not Found"}`, 404, "public repository", ""},
			{"NFR-PRIV-1: a private repo is refused, even with admin rights", 200, private, 403, "private", ""},
			{"GitHub refuses, as for an organization that has not approved the app", 403, `{"message":"OAuth App access restrictions"}`, 403, "approve Office Hours", ""},
			{"GitHub no longer accepts the user's token", 401, `{"message":"Bad credentials"}`, 401, "Sign in again", ""},
			{"A-7: a reply without permissions proves nothing", 200, githubRepo("mia-k", "User", "stash", ""), 502, "rights", ""},
			{"GitHub is down", 500, `{"message":"Server Error"}`, 502, "GitHub didn't answer", ""},
			{"GitHub sends a page instead of JSON", 200, `<html>`, 502, "GitHub didn't answer", ""},
			{"NFR-SEC-3: a name from GitHub that is no repo name is never stored", 200, githubRepo("mia-k", "User", "st ash", adminRights), 502, "GitHub didn't answer", ""},
		} {
			a := newProjectsApp(t)
			a.gh.status, a.gh.reply = tt.status, tt.reply
			res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
			refused(t, tt.name, res, body, tt.want, tt.says)
			if tt.never != "" && strings.Contains(body, tt.never) {
				t.Errorf("%s: the sentence must not say %q: %s", tt.name, tt.never, body)
			}
			if len(*a.events) != 0 {
				t.Errorf("%s: a refused registration still did %q", tt.name, *a.events)
			}
		}
	})

	t.Run("A-7 is logged by name, and a GitHub that hangs is given up on", func(t *testing.T) {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(os.Stderr)

		a := newProjectsApp(t)
		a.gh.reply = githubRepo("mia-k", "User", "stash", "")
		a.register("mia-k", "stash", a.as(t, "gh42"))
		if !strings.Contains(buf.String(), "A-7") {
			t.Errorf("the log does not name assumption A-7: %q", buf.String())
		}

		a = newProjectsApp(t)
		a.gh.hang = make(chan struct{})
		defer close(a.gh.hang)
		start := time.Now()
		res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		refused(t, "timeout", res, body, 502, "GitHub didn't answer")
		if waited := time.Since(start); waited > 3*time.Second {
			t.Errorf("waited %v for a hanging GitHub", waited)
		}
		if len(*a.events) != 0 {
			t.Errorf("a timed-out registration still did %q", *a.events)
		}
		// NFR-OPS-2: the timeout is logged with the address, never with the token.
		if strings.Contains(buf.String(), testUserToken) {
			t.Errorf("NFR-SEC-1: the log contains the user's GitHub token: %q", buf.String())
		}
	})

	t.Run("NFR-SEC-3: a name that is not owner/repo is refused before GitHub is asked", func(t *testing.T) {
		for name, body := range map[string]string{
			"no owner":                 `{"repo":"stash"}`,
			"no repo":                  `{"owner":"paperplane"}`,
			"nothing":                  `{}`,
			"owner with an underscore": `{"owner":"paper_plane","repo":"stash"}`,
			"owner of 40 characters":   `{"owner":"` + strings.Repeat("a", 40) + `","repo":"stash"}`,
			"repo of 101 characters":   `{"owner":"paperplane","repo":"` + strings.Repeat("r", 101) + `"}`,
			"repo is .":                `{"owner":"paperplane","repo":"."}`,
			"repo is ..":               `{"owner":"paperplane","repo":".."}`,
			"a path as the repo":       `{"owner":"paperplane","repo":"stash/../../user"}`,
			"markup":                   `{"owner":"paperplane","repo":"<b>"}`,
			"both in one field":        `{"owner":"paperplane/stash","repo":""}`,
			"a number":                 `{"owner":1,"repo":"stash"}`,
			"a field we don't have":    `{"owner":"paperplane","repo":"stash","id":"p1"}`,
		} {
			a := newProjectsApp(t)
			res, text := call(a.mux, "POST", "/api/projects", body, a.as(t, "gh42"))
			if res.StatusCode != 400 || !strings.Contains(text, `"ok":false`) {
				t.Errorf("%s: status %d body %s, want 400", name, res.StatusCode, text)
			}
			if len(a.gh.asked) != 0 || len(*a.events) != 0 {
				t.Errorf("%s: GitHub asked %q, events %q; want neither", name, a.gh.asked, *a.events)
			}
		}
	})

	t.Run("NFR-SEC-2, NFR-SEC-5: only a signed-in user on our own pages", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			signedIn bool
			mutate   func(*http.Request)
			want     int
		}{
			{"signed out", false, func(*http.Request) {}, 401},
			{"another site's origin", true, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, 403},
			{"no Origin", true, func(r *http.Request) { r.Header.Del("Origin") }, 403},
			{"a form post", true, func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }, 415},
		} {
			a := newProjectsApp(t)
			var cookie *http.Cookie
			if tt.signedIn {
				cookie = a.as(t, "gh42")
			}
			res, body := call(a.mux, "POST", "/api/projects", `{"owner":"paperplane","repo":"stash"}`, cookie, tt.mutate)
			if res.StatusCode != tt.want || len(a.gh.asked) != 0 || len(*a.events) != 0 {
				t.Errorf("%s: status %d (want %d), GitHub asked %q, events %q: %s", tt.name, res.StatusCode, tt.want, a.gh.asked, *a.events, body)
			}
		}
	})

	t.Run("plan decision 3: when the chat service fails, no project is recorded", func(t *testing.T) {
		for name, breakIt := range map[string]func(*fakeRooms){
			"the group can't be created":   func(f *fakeRooms) { f.createErr = errors.New("nexconn create group: http 500, code 1050: internal") },
			"the registrant can't join it": func(f *fakeRooms) { f.joinErr = errors.New("nexconn join group: http 500, code 1050: internal") },
		} {
			a := newProjectsApp(t)
			breakIt(a.rooms)
			res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
			refused(t, name, res, body, 502, "chat room")
			if strings.Contains(body, "nexconn") || strings.Contains(body, "1050") {
				t.Errorf("%s: NFR-OPS-2: Nexconn's error text reached the user: %s", name, body)
			}
			if len(a.projects.byRepo) != 0 {
				t.Errorf("%s: a project was recorded without a room: %+v", name, a.projects.byRepo)
			}
		}

		// The same when joining a project that exists: no maintainer row without the room.
		a := newProjectsApp(t)
		a.register("paperplane", "stash", a.as(t, "gh42"))
		a.rooms.joinErr = errors.New("nexconn join group: http 500")
		res, body := a.register("paperplane", "stash", a.as(t, "gh7"))
		refused(t, "second maintainer", res, body, 502, "chat room")
		if p := a.projects.seenBy(a.projects.byRepo["paperplane/stash"], "gh7"); p.IsMaintainer || p.IsMember {
			t.Errorf("gh7 became %+v without being in the room", p)
		}
	})

	t.Run("a session whose user row is gone is asked to sign in again", func(t *testing.T) {
		a := newProjectsApp(t)
		a.projects.createErr = store.ErrUnknownUser
		res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		refused(t, "unknown user", res, body, 401, "Sign in again")
	})

	t.Run("NFR-OPS-2: a database failure is a 500 without its text", func(t *testing.T) {
		a := newProjectsApp(t)
		a.projects.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body := a.register("paperplane", "stash", a.as(t, "gh42"))
		refused(t, "database down", res, body, 500, "Try again")
		if strings.Contains(body, "10.0.0.5") || len(*a.events) != 0 {
			t.Errorf("body %s, events %q: nothing may leak or be created", body, *a.events)
		}
	})

	t.Run("a server without GitHub, a database or Nexconn says it is not set up", func(t *testing.T) {
		for name, strip := range map[string]func(*deps){
			"no GitHub app": func(d *deps) { d.github = nil },
			"no database":   func(d *deps) { d.projects = nil },
			"no Nexconn":    func(d *deps) { d.rooms = nil },
		} {
			a := newProjectsApp(t)
			d := a.deps
			strip(&d)
			res, body := call(newMux(config{AppURL: testSite}, d), "POST", "/api/projects", `{"owner":"paperplane","repo":"stash"}`, a.as(t, "gh42"))
			refused(t, name, res, body, 503, "set up")
		}
	})
}

func TestProjectPage(t *testing.T) {
	a := newProjectsApp(t)
	var mine projectReply
	_, body := a.register("paperplane", "stash", a.as(t, "gh42"))
	decode(t, body, &mine)

	t.Run("FR-PROJ-3: anyone can read a project's public facts, signed out", func(t *testing.T) {
		res, body := call(a.mux, "GET", "/api/p/paperplane/stash", "", nil)
		var p projectReply
		decode(t, body, &p)
		want := projectReply{Owner: "PaperPlane", Repo: "Stash", Description: "A tiny HTTP cache."}
		if res.StatusCode != 200 || p != want {
			t.Fatalf("status %d project %+v, want %+v", res.StatusCode, p, want)
		}
		// The id names the chat room. A visitor has no use for it.
		if strings.Contains(body, mine.ID) || strings.Contains(body, `"id"`) {
			t.Fatalf("a signed-out reply carries the project id: %s", body)
		}
	})

	t.Run("a signed-in caller also gets the id and their own role", func(t *testing.T) {
		for _, tt := range []struct {
			user                   string
			isMember, isMaintainer bool
		}{{"gh42", true, true}, {"gh7", false, false}} {
			_, body := call(a.mux, "GET", "/api/p/PaperPlane/STASH", "", a.as(t, tt.user))
			var p projectReply
			decode(t, body, &p)
			want := projectReply{ID: mine.ID, Owner: "PaperPlane", Repo: "Stash", Description: "A tiny HTTP cache.", IsMember: tt.isMember, IsMaintainer: tt.isMaintainer}
			if p != want {
				t.Errorf("%s: %+v, want %+v", tt.user, p, want)
			}
		}
	})

	t.Run("a repo nobody registered, or a name that is none, is a 404 with a sentence", func(t *testing.T) {
		for _, path := range []string{"/api/p/paperplane/other", "/api/p/paper_plane/stash", "/api/p/paperplane/..", "/api/p/paperplane/%3Cb%3E"} {
			res, body := call(a.mux, "GET", path, "", nil)
			// "/.." is cleaned away by the router before any handler runs, which is as good.
			if res.StatusCode != 404 && res.StatusCode != http.StatusMovedPermanently {
				t.Errorf("%s: status %d body %s, want 404", path, res.StatusCode, body)
			}
			if strings.Contains(body, "<b>") {
				t.Errorf("%s: NFR-SEC-6: the name came back unescaped: %s", path, body)
			}
		}
		res, body := call(a.mux, "GET", "/api/p/paperplane/other", "", nil)
		refused(t, "not registered", res, body, 404, "paperplane/other")
	})

	t.Run("no database is a 503, a failing one a 500", func(t *testing.T) {
		d := a.deps
		d.projects = nil
		res, body := call(newMux(config{AppURL: testSite}, d), "GET", "/api/p/paperplane/stash", "", nil)
		refused(t, "no database", res, body, 503, "set up")

		broken := newProjectsApp(t)
		broken.projects.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body = call(broken.mux, "GET", "/api/p/paperplane/stash", "", nil)
		refused(t, "database down", res, body, 500, "Try again")
		if strings.Contains(body, "10.0.0.5") {
			t.Errorf("NFR-OPS-2: the database error reached the user: %s", body)
		}
	})
}

func TestMyProjects(t *testing.T) {
	a := newProjectsApp(t)
	mia, diego := a.as(t, "gh42"), a.as(t, "gh7")

	t.Run("NFR-SEC-2: signed out is a 401", func(t *testing.T) {
		res, body := call(a.mux, "GET", "/api/me/projects", "", nil)
		refused(t, "signed out", res, body, 401, "Sign in")
	})

	t.Run("someone with no projects gets an empty list, not null", func(t *testing.T) {
		res, body := call(a.mux, "GET", "/api/me/projects", "", mia)
		if res.StatusCode != 200 || !strings.Contains(body, `"data":[]`) {
			t.Fatalf("status %d body %s, want 200 with an empty list", res.StatusCode, body)
		}
	})

	t.Run("FR-PROJ-2: lists the caller's projects with their role, and nobody else's", func(t *testing.T) {
		a.register("paperplane", "stash", mia)
		a.gh.reply = githubRepo("diego", "User", "atlas", adminRights)
		a.register("diego", "atlas", diego)
		a.gh.reply = githubRepo("mia-k", "User", "notes", adminRights)
		a.register("mia-k", "notes", mia)

		_, body := call(a.mux, "GET", "/api/me/projects", "", mia)
		var list []projectReply
		decode(t, body, &list)
		if len(list) != 2 || list[0].Owner != "mia-k" || list[1].Owner != "PaperPlane" {
			t.Fatalf("mia's list = %+v, want mia-k/notes then PaperPlane/Stash", list)
		}
		for _, p := range list {
			if p.ID == "" || !p.IsMaintainer || !p.IsMember {
				t.Errorf("project %+v: want an id and both roles", p)
			}
		}
	})

	t.Run("no database is a 503, a failing one a 500", func(t *testing.T) {
		d := a.deps
		d.projects = nil
		res, body := call(newMux(config{AppURL: testSite}, d), "GET", "/api/me/projects", "", mia)
		refused(t, "no database", res, body, 503, "set up")

		broken := newProjectsApp(t)
		broken.projects.err = errors.New("connection refused to 10.0.0.5:5432")
		res, body = call(broken.mux, "GET", "/api/me/projects", "", broken.as(t, "gh42"))
		refused(t, "database down", res, body, 500, "Try again")
	})
}
