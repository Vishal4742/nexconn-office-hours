package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// statsFixture is two projects after a few sessions, built so that the five numbers differ in
// each project and on the whole site: a number in the wrong column, or counted for the wrong
// project, shows.
//
//	stash   2 sessions, 11 hands of every status, 4 answered, 4 entries (1 deleted), 5 people
//	atlas   1 session,   5 hands,                 3 answered, 3 entries (1 deleted), 4 people
//
// Ana (gh3) is in both rooms. Zoe (gh9) signed in and joined none.
func statsFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES
		('gh1', 1, 'mia'), ('gh2', 2, 'diego'), ('gh3', 3, 'ana'), ('gh4', 4, 'noor'), ('gh5', 5, 'li'),
		('gh6', 6, 'sam'), ('gh7', 7, 'kofi'), ('gh8', 8, 'ines'), ('gh9', 9, 'zoe')`)
	mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES
		('stash', 1001, 'paperplane', 'stash', 'gh1'), ('atlas', 1002, 'ana', 'atlas', 'gh3')`)
	// A maintainer is a member too, and is one participant, not two.
	mustExec(t, pool, `INSERT INTO maintainers (project_id, user_id) VALUES ('stash', 'gh1'), ('atlas', 'gh3')`)
	mustExec(t, pool, `INSERT INTO members (project_id, user_id) VALUES
		('stash', 'gh1'), ('stash', 'gh2'), ('stash', 'gh3'), ('stash', 'gh4'), ('stash', 'gh5'),
		('atlas', 'gh3'), ('atlas', 'gh6'), ('atlas', 'gh7'), ('atlas', 'gh8')`)
	mustExec(t, pool, `INSERT INTO sessions (id, project_id, title, status, ended_at) OVERRIDING SYSTEM VALUE VALUES
		(1, 'stash', 'Stash, week 1', 'ended', now()),
		(2, 'stash', 'Stash, week 2', 'live', NULL),
		(3, 'atlas', 'Atlas kickoff', 'ended', now())`)
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, status) VALUES
		(1, 'gh2', 'How do I retry only on 5xx?', 'answered'),
		(1, 'gh3', 'Can two processes share the cache?', 'answered'),
		(1, 'gh2', 'Where are the docs?', 'answered'),
		(1, 'gh4', 'Is there a Redis backend?', 'skipped'),
		(1, 'gh5', 'Never mind, I found it', 'withdrawn'),
		(1, 'gh5', 'Buy cheap watches', 'removed'),
		(1, 'gh4', 'Retries on 5xx, the same question', 'merged'),
		(1, 'gh3', 'What about HTTP/3?', 'not_reached'),
		(2, 'gh5', 'How do I set a TTL?', 'answered'),
		(2, 'gh2', 'Does it work on Windows?', 'answering'),
		(2, 'gh3', 'Can I plug in my own store?', 'waiting'),
		(3, 'gh6', 'How do I add a map layer?', 'answered'),
		(3, 'gh7', 'Which projections are supported?', 'answered'),
		(3, 'gh8', 'Is there a command line tool?', 'answered'),
		(3, 'gh6', 'Can it draw offline tiles?', 'carried'),
		(3, 'gh7', 'Does it read shapefiles?', 'not_reached')`)
	// One entry per answered hand. A maintainer deleted one in each project.
	mustExec(t, pool, `INSERT INTO faq_entries (project_id, slug, title, answer, author_id, deleted_at) VALUES
		('stash', 'retry-only-on-5xx', 'How do I retry only on 5xx?', 'Use retryIf.', 'gh1', NULL),
		('stash', 'share-the-cache', 'Can two processes share the cache?', 'Yes, through a file lock.', 'gh1', NULL),
		('stash', 'where-are-the-docs', 'Where are the docs?', 'They moved.', 'gh1', now()),
		('stash', 'set-a-ttl', 'How do I set a TTL?', 'Pass ttl to New.', 'gh1', NULL),
		('atlas', 'add-a-map-layer', 'How do I add a map layer?', 'Call AddLayer.', 'gh3', NULL),
		('atlas', 'projections', 'Which projections are supported?', 'Every one PROJ knows.', 'gh3', NULL),
		('atlas', 'command-line-tool', 'Is there a command line tool?', 'Not any more.', 'gh3', now())`)
}

func TestStats(t *testing.T) {
	pool := testDB(t)
	statsFixture(t, pool)
	s, ctx := New(pool), context.Background()

	for _, tt := range []struct {
		name      string
		projectID string
		want      Stats
	}{
		{
			"FR-MET-1: a project's sessions, every hand whatever became of it, the answered ones, the entries not deleted, its people",
			"stash", Stats{Sessions: 2, HandsRaised: 11, HandsAnswered: 4, FAQEntries: 3, Participants: 5},
		},
		{
			"FR-MET-1: another project has numbers of its own",
			"atlas", Stats{Sessions: 1, HandsRaised: 5, HandsAnswered: 3, FAQEntries: 2, Participants: 4},
		},
		{
			"FR-MET-1: the whole site adds the projects up, and someone in two rooms is one participant",
			"", Stats{Sessions: 3, HandsRaised: 16, HandsAnswered: 7, FAQEntries: 5, Participants: 8},
		},
		{
			"a project nobody registered has nothing but zeros",
			"no-such-project", Stats{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Stats(ctx, tt.projectID)
			if err != nil || got != tt.want {
				t.Fatalf("Stats(%q) = %+v, %v; want %+v", tt.projectID, got, err, tt.want)
			}
		})
	}
}

func TestStatsBeforeAnySession(t *testing.T) {
	pool := testDB(t)
	s, ctx := New(pool), context.Background()

	t.Run("FR-MET-1: a site nobody uses yet counts zeros", func(t *testing.T) {
		if got, err := s.Stats(ctx, ""); err != nil || got != (Stats{}) {
			t.Fatalf("Stats of an empty database = %+v, %v; want zeros", got, err)
		}
	})

	t.Run("FR-MET-1: a project registered a moment ago has its maintainer as its one participant", func(t *testing.T) {
		mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES ('gh1', 1, 'mia')`)
		mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('stash', 1001, 'paperplane', 'stash', 'gh1')`)
		mustExec(t, pool, `INSERT INTO maintainers (project_id, user_id) VALUES ('stash', 'gh1')`)
		mustExec(t, pool, `INSERT INTO members (project_id, user_id) VALUES ('stash', 'gh1')`)
		for _, projectID := range []string{"stash", ""} {
			if got, err := s.Stats(ctx, projectID); err != nil || got != (Stats{Participants: 1}) {
				t.Errorf("Stats(%q) = %+v, %v; want one participant and nothing else", projectID, got, err)
			}
		}
	})
}

func TestStatsFailsRatherThanCountZeros(t *testing.T) {
	pool := testDB(t)
	statsFixture(t, pool)
	// A page that shows five zeros because the database could not be asked would be a lie.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := New(pool).Stats(ctx, "stash")
	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "store: ") || got != (Stats{}) {
		t.Fatalf("Stats on a database that cannot be asked = %+v, %v; want no numbers and the store's own error around the cause", got, err)
	}
}
