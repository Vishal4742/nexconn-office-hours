package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// people adds the three users the project tests act as.
func people(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES ('gh1', 1, 'mia'), ('gh2', 2, 'diego'), ('gh3', 3, 'ana')`)
}

// count runs a query that returns one number.
func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func TestCreateProject(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()

	stash := Project{ID: "aaaa", GitHubID: 1001, Owner: "PaperPlane", Repo: "Stash", Description: "A tiny HTTP cache."}
	got, created, err := s.CreateProject(ctx, stash, "gh1")
	if err != nil || !created {
		t.Fatalf("CreateProject = created %v, err %v; want a new project", created, err)
	}
	want := stash
	want.IsMember, want.IsMaintainer = true, true
	if got != want {
		t.Fatalf("CreateProject = %+v, want %+v", got, want)
	}
	// FR-PROJ-2: the registrant is the first maintainer, and a member of the room.
	if n := count(t, pool, `SELECT count(*) FROM maintainers WHERE project_id = 'aaaa' AND user_id = 'gh1'`); n != 1 {
		t.Fatalf("FR-PROJ-2: maintainers rows for the registrant = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM members WHERE project_id = 'aaaa' AND user_id = 'gh1'`); n != 1 {
		t.Fatalf("FR-PROJ-2: members rows for the registrant = %d, want 1", n)
	}

	// FR-PROJ-2: the same repository again, in another letter case and by someone else, changes
	// nothing and answers with the project that exists. The asker gets no role from it.
	again, created, err := s.CreateProject(ctx, Project{ID: "bbbb", GitHubID: 2002, Owner: "paperplane", Repo: "STASH", Description: "Changed."}, "gh2")
	if err != nil || created {
		t.Fatalf("second CreateProject = created %v, err %v; want the existing project", created, err)
	}
	if again != stash {
		t.Fatalf("second CreateProject = %+v, want the first project with no role for the asker: %+v", again, stash)
	}
	if projects, maintainers, members := count(t, pool, `SELECT count(*) FROM projects`), count(t, pool, `SELECT count(*) FROM maintainers`), count(t, pool, `SELECT count(*) FROM members`); projects != 1 || maintainers != 1 || members != 1 {
		t.Fatalf("after the second registration: %d projects, %d maintainers, %d members; want 1 of each", projects, maintainers, members)
	}

	// FR-PROJ-2: the same repository under a new name (it was renamed on GitHub) is still that one
	// project. GitHub's id says so.
	renamed, created, err := s.CreateProject(ctx, Project{ID: "dddd", GitHubID: 1001, Owner: "paperplane", Repo: "cache"}, "gh1")
	if err != nil || created {
		t.Fatalf("CreateProject for a renamed repo = created %v, err %v; want the existing project", created, err)
	}
	if renamed.ID != "aaaa" || renamed.Repo != "Stash" || !renamed.IsMaintainer {
		t.Fatalf("CreateProject for a renamed repo = %+v, want project aaaa under its registered name", renamed)
	}
	if n := count(t, pool, `SELECT count(*) FROM projects`); n != 1 {
		t.Fatalf("%d projects after registering a renamed repo, want 1", n)
	}

	// A session can outlive its user row (an account deleted here, a database reset in development).
	// That must be told apart from a broken database: the person only has to sign in again.
	_, _, err = s.CreateProject(ctx, Project{ID: "cccc", GitHubID: 3003, Owner: "ghost", Repo: "town"}, "gh404")
	if !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("CreateProject for a user with no row: err = %v, want ErrUnknownUser", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM projects WHERE id = 'cccc'`); n != 0 {
		t.Fatalf("a refused registration left %d project rows behind", n)
	}
}

func TestCreateProjectAtTheSameMoment(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()

	// FR-PROJ-2: eight registrations of one repository at once, each under its own id.
	const racers = 8
	type result struct {
		id      string
		created bool
		err     error
	}
	results := make([]result, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			p, created, err := s.CreateProject(ctx, Project{ID: fmt.Sprintf("racer%d", i), GitHubID: 1001, Owner: "paperplane", Repo: "stash"}, "gh1")
			results[i] = result{p.ID, created, err}
		})
	}
	wg.Wait()

	winners := 0
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("racer %d: %v", i, r.err)
		}
		if r.created {
			winners++
		}
		if r.id != results[0].id {
			t.Fatalf("racer %d got project %q, racer 0 got %q: they must all get the same one", i, r.id, results[0].id)
		}
	}
	if winners != 1 {
		t.Fatalf("%d registrations created a project, want exactly 1", winners)
	}
	if projects, maintainers := count(t, pool, `SELECT count(*) FROM projects`), count(t, pool, `SELECT count(*) FROM maintainers`); projects != 1 || maintainers != 1 {
		t.Fatalf("%d projects and %d maintainers rows, want 1 and 1", projects, maintainers)
	}
}

func TestProjectByRepo(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()

	if _, found, err := s.ProjectByRepo(ctx, "paperplane", "stash", ""); err != nil || found {
		t.Fatalf("before registration: found %v, err %v; want not found", found, err)
	}
	stash := Project{ID: "aaaa", GitHubID: 1001, Owner: "PaperPlane", Repo: "Stash", Description: "A tiny HTTP cache."}
	if _, _, err := s.CreateProject(ctx, stash, "gh1"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	mustExec(t, pool, `INSERT INTO members (project_id, user_id) VALUES ('aaaa', 'gh2')`) // diego joined the room

	for _, tt := range []struct {
		name                   string
		owner, repo, viewer    string
		isMember, isMaintainer bool
	}{
		{"the registrant", "PaperPlane", "Stash", "gh1", true, true},
		{"FR-PROJ-2: any letter case finds it", "paperPLANE", "stash", "gh1", true, true},
		{"someone who joined is a member, not a maintainer", "paperplane", "stash", "gh2", true, false},
		{"a stranger", "paperplane", "stash", "gh3", false, false},
		{"FR-PROJ-3: nobody (a signed-out visitor)", "paperplane", "stash", "", false, false},
	} {
		got, found, err := s.ProjectByRepo(ctx, tt.owner, tt.repo, tt.viewer)
		if err != nil || !found {
			t.Fatalf("%s: found %v, err %v", tt.name, found, err)
		}
		want := stash // always GitHub's own spelling, whatever was asked
		want.IsMember, want.IsMaintainer = tt.isMember, tt.isMaintainer
		if got != want {
			t.Errorf("%s: %+v, want %+v", tt.name, got, want)
		}
	}

	if _, found, err := s.ProjectByRepo(ctx, "paperplane", "other", "gh1"); err != nil || found {
		t.Fatalf("another repo of the same owner: found %v, err %v; want not found", found, err)
	}
}

func TestAddMaintainer(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()
	if _, _, err := s.CreateProject(ctx, Project{ID: "aaaa", GitHubID: 1001, Owner: "paperplane", Repo: "stash"}, "gh1"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	mustExec(t, pool, `INSERT INTO members (project_id, user_id) VALUES ('aaaa', 'gh3')`) // ana joined earlier

	// Plan decision 4: a second maintainer of the repo becomes a maintainer and a member here.
	// Doing it twice, or for someone who is already a member, adds nothing twice.
	for _, user := range []string{"gh2", "gh2", "gh3"} {
		if err := s.AddMaintainer(ctx, "aaaa", user); err != nil {
			t.Fatalf("AddMaintainer(%s): %v", user, err)
		}
	}
	for _, user := range []string{"gh1", "gh2", "gh3"} {
		p, _, err := s.ProjectByRepo(ctx, "paperplane", "stash", user)
		if err != nil || !p.IsMember || !p.IsMaintainer {
			t.Errorf("%s: member %v, maintainer %v, err %v; want both", user, p.IsMember, p.IsMaintainer, err)
		}
	}
	if maintainers, members := count(t, pool, `SELECT count(*) FROM maintainers`), count(t, pool, `SELECT count(*) FROM members`); maintainers != 3 || members != 3 {
		t.Fatalf("%d maintainers and %d members rows, want 3 and 3", maintainers, members)
	}

	if err := s.AddMaintainer(ctx, "aaaa", "gh404"); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("AddMaintainer for a user with no row: err = %v, want ErrUnknownUser", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM members WHERE user_id = 'gh404'`); n != 0 {
		t.Fatalf("a refused AddMaintainer left %d members rows behind", n)
	}
}

func TestProjectsOf(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()

	for _, p := range []struct {
		project Project
		by      string
	}{
		{Project{ID: "p-stash", GitHubID: 1, Owner: "PaperPlane", Repo: "stash", Description: "A tiny HTTP cache."}, "gh1"},
		{Project{ID: "p-atlas", GitHubID: 2, Owner: "ana", Repo: "Atlas"}, "gh3"},
		{Project{ID: "p-zed", GitHubID: 3, Owner: "paperplane", Repo: "Zed"}, "gh1"},
		{Project{ID: "p-other", GitHubID: 4, Owner: "diego", Repo: "other"}, "gh2"},
	} {
		if _, _, err := s.CreateProject(ctx, p.project, p.by); err != nil {
			t.Fatalf("CreateProject(%s): %v", p.project.ID, err)
		}
	}
	mustExec(t, pool, `INSERT INTO members (project_id, user_id) VALUES ('p-atlas', 'gh1')`) // mia joined ana's room

	// Mia's dashboard: the two she maintains and the one she joined, by name whatever the letter case.
	got, err := s.ProjectsOf(ctx, "gh1")
	if err != nil {
		t.Fatalf("ProjectsOf: %v", err)
	}
	want := []Project{
		{ID: "p-atlas", GitHubID: 2, Owner: "ana", Repo: "Atlas", IsMember: true},
		{ID: "p-stash", GitHubID: 1, Owner: "PaperPlane", Repo: "stash", Description: "A tiny HTTP cache.", IsMember: true, IsMaintainer: true},
		{ID: "p-zed", GitHubID: 3, Owner: "paperplane", Repo: "Zed", IsMember: true, IsMaintainer: true},
	}
	if len(got) != len(want) {
		t.Fatalf("ProjectsOf = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("project %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	none, err := s.ProjectsOf(ctx, "gh404")
	if err != nil || len(none) != 0 {
		t.Fatalf("ProjectsOf for someone with no rooms = %+v, %v; want none", none, err)
	}
}

func TestAddMemberAndMembers(t *testing.T) {
	pool := testDB(t)
	s, ctx := New(pool), context.Background()
	// Mia has a display name; Diego's row has none, as rows written by hand may.
	mustExec(t, pool, `INSERT INTO users (id, github_id, login, name, avatar_url) VALUES
		('gh1', 1, 'mia-k', 'Mia K', 'https://avatars.example/u/1'), ('gh2', 2, 'diego', '', ''), ('gh3', 3, 'ana', 'Ana', '')`)
	if _, _, err := s.CreateProject(ctx, Project{ID: "aaaa", GitHubID: 1001, Owner: "paperplane", Repo: "stash"}, "gh1"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, _, err := s.CreateProject(ctx, Project{ID: "bbbb", GitHubID: 1002, Owner: "ana", Repo: "atlas"}, "gh3"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// FR-PROJ-4: joining twice is one row, and gives no maintainer role.
	for range 2 {
		if err := s.AddMember(ctx, "aaaa", "gh2"); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM members WHERE project_id = 'aaaa' AND user_id = 'gh2'`); n != 1 {
		t.Fatalf("members rows after joining twice = %d, want 1", n)
	}
	if p, _, err := s.ProjectByRepo(ctx, "paperplane", "stash", "gh2"); err != nil || !p.IsMember || p.IsMaintainer {
		t.Fatalf("after joining: member %v, maintainer %v, err %v; want a member only", p.IsMember, p.IsMaintainer, err)
	}

	// The room's people, in the order they came, each with a name the chat can show.
	got, err := s.Members(ctx, "aaaa")
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	want := []Member{
		{UserID: "gh1", Name: "Mia K", AvatarURL: "https://avatars.example/u/1"},
		{UserID: "gh2", Name: "diego"}, // no display name: the login stands in, Chat UI needs a name
	}
	if len(got) != len(want) {
		t.Fatalf("Members = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("member %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Another room's people are not in it, and a room nobody is in is empty.
	if other, err := s.Members(ctx, "bbbb"); err != nil || len(other) != 1 || other[0].UserID != "gh3" {
		t.Fatalf("Members of the other room = %+v, %v", other, err)
	}
	if none, err := s.Members(ctx, "no-such-project"); err != nil || len(none) != 0 {
		t.Fatalf("Members of no project = %+v, %v; want none", none, err)
	}

	if err := s.AddMember(ctx, "aaaa", "gh404"); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("AddMember for a user with no row: err = %v, want ErrUnknownUser", err)
	}
}

func TestProjectOf(t *testing.T) {
	pool := testDB(t)
	people(t, pool)
	s, ctx := New(pool), context.Background()
	for _, p := range []Project{
		{ID: "aaaa", GitHubID: 1001, Owner: "PaperPlane", Repo: "Stash"},
		{ID: "bbbb", GitHubID: 2002, Owner: "paperplane", Repo: "cache"},
	} {
		if _, _, err := s.CreateProject(ctx, p, "gh1"); err != nil {
			t.Fatalf("CreateProject(%s): %v", p.ID, err)
		}
	}

	for _, tt := range []struct {
		name        string
		githubID    int64
		owner, repo string
		wantID      string // "" for not found
	}{
		{"by GitHub's id and its name", 1001, "paperplane", "stash", "aaaa"},
		{"by GitHub's id under a name nobody registered: the repo was renamed", 1001, "paperplane", "vault", "aaaa"},
		{"by name when GitHub's id is new here: the name belongs to another repo's project", 3003, "PAPERPLANE", "STASH", "aaaa"},
		{"the id wins over the name: a renamed repo that now carries another project's old name", 2002, "paperplane", "stash", "bbbb"},
		{"neither", 3003, "paperplane", "vault", ""},
	} {
		got, found, err := s.ProjectOf(ctx, tt.githubID, tt.owner, tt.repo, "gh1")
		if err != nil || found != (tt.wantID != "") || got.ID != tt.wantID {
			t.Errorf("%s: project %q, found %v, err %v; want %q", tt.name, got.ID, found, err, tt.wantID)
		}
		if found && (!got.IsMember || !got.IsMaintainer) {
			t.Errorf("%s: the registrant's role is missing: %+v", tt.name, got)
		}
	}
}
