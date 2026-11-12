package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"officehours/migrations"
)

// testDB gives a test its own schema in the database TEST_DATABASE_URL names, with every migration
// applied, and drops it at the end. These tests are about what Postgres refuses, so nothing is faked.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI: these tests may not be skipped there")
		}
		t.Skip("set TEST_DATABASE_URL to run this against a real Postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := Open(ctx, base)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	schema := fmt.Sprintf("t_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		admin.Close()
	})

	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	pool, err := Open(ctx, base+sep+"search_path="+schema)
	if err != nil {
		t.Fatalf("Open with search_path: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return pool
}

// mustExec runs setup SQL that has to work.
func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("setup %q: %v", sql, err)
	}
}

// isUniqueViolation reports whether Postgres refused a row because a unique rule was broken.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// seed adds two users, one project and one live session, the base every constraint test needs.
func seed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login) VALUES ('gh1', 1, 'mia'), ('gh2', 2, 'diego')`)
	mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj1', 1, 'paperplane', 'stash', 'gh1')`)
	mustExec(t, pool, `INSERT INTO sessions (project_id, title) VALUES ('proj1', 'Thursday office hours')`)
}

func TestMigrateAppliesEachFileOnce(t *testing.T) {
	pool := testDB(t) // the first run happens in here
	applied, err := Migrate(context.Background(), pool, migrations.FS)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("second Migrate applied %v, want nothing", applied)
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n == 0 {
		t.Fatal("schema_migrations is empty after migrating")
	}
}

func TestAFailedMigrationChangesNothingAndIsTriedAgain(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	broken := fstest.MapFS{
		"900_ok.sql":     {Data: []byte(`CREATE TABLE kept (id INT)`)},
		"901_broken.sql": {Data: []byte(`CREATE TABLE half (id INT); SELECT no_such_function()`)},
	}
	applied, err := Migrate(ctx, pool, broken)
	if err == nil {
		t.Fatal("Migrate with a broken file = nil error, want one")
	}
	if len(applied) != 1 || applied[0] != "900_ok.sql" {
		t.Fatalf("applied = %v, want only the file before the broken one", applied)
	}
	// The broken file's own table and its record are both gone: the transaction was rolled back.
	var half, recorded bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('half') IS NOT NULL,
		EXISTS (SELECT 1 FROM schema_migrations WHERE name = '901_broken.sql')`).Scan(&half, &recorded); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if half || recorded {
		t.Fatalf("after the failure: table exists = %v, recorded = %v, want neither", half, recorded)
	}
	// Once the file is fixed, the next run applies it and nothing else.
	broken["901_broken.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE half (id INT)`)}
	applied, err = Migrate(ctx, pool, broken)
	if err != nil || len(applied) != 1 || applied[0] != "901_broken.sql" {
		t.Fatalf("after the fix: applied = %v, err = %v, want only the fixed file", applied, err)
	}
}

func TestTheDatabaseEnforcesTheRules(t *testing.T) {
	tests := []struct {
		name    string
		setup   []string // must succeed
		allowed string   // must succeed too, when set: the rule is no wider than it should be
		refused string   // must fail with a unique violation
	}{
		{
			name:    "FR-SESS-2: one live session per project",
			allowed: `INSERT INTO sessions (project_id, title, status, ended_at) VALUES ('proj1', 'Last week', 'ended', now())`,
			refused: `INSERT INTO sessions (project_id, title) VALUES ('proj1', 'A second live one')`,
		},
		{
			name: "FR-HAND-2: one open hand per user per session",
			setup: []string{
				`INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh2', 'Withdrawn earlier', 'withdrawn')`,
				`INSERT INTO hands (session_id, author_id, title) VALUES (1, 'gh2', 'How do I retry only on 5xx errors?')`,
			},
			allowed: `INSERT INTO hands (session_id, author_id, title) VALUES (1, 'gh1', 'Another user may wait too')`,
			refused: `INSERT INTO hands (session_id, author_id, title) VALUES (1, 'gh2', 'A second open hand')`,
		},
		{
			name: "FR-QUEUE-3: one hand answered at a time",
			setup: []string{
				`INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh1', 'Being answered', 'answering')`,
			},
			allowed: `INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh2', 'Answered before', 'answered')`,
			refused: `INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh2', 'Also being answered', 'answering')`,
		},
		{
			name: "FR-FAQ-1: a slug is unique inside its project",
			setup: []string{
				`INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj2', 2, 'paperplane', 'other', 'gh1')`,
				`INSERT INTO faq_entries (project_id, slug, title, answer, author_id) VALUES ('proj1', 'retry-on-5xx', 'Retry on 5xx', 'Use retryIf.', 'gh1')`,
			},
			allowed: `INSERT INTO faq_entries (project_id, slug, title, answer, author_id) VALUES ('proj2', 'retry-on-5xx', 'Retry on 5xx', 'Use retryIf.', 'gh1')`,
			refused: `INSERT INTO faq_entries (project_id, slug, title, answer, author_id) VALUES ('proj1', 'retry-on-5xx', 'Again', 'Same slug.', 'gh1')`,
		},
		{
			name:    "FR-PROJ-2: a GitHub repository is one project whatever it is called now",
			allowed: `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj5', 5, 'paperplane', 'cache', 'gh1')`,
			refused: `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj6', 1, 'paperplane', 'renamed-stash', 'gh1')`,
		},
		{
			name:    "FR-PROJ-2: a repo is one project whatever its letter case",
			allowed: `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj3', 3, 'paperplane', 'stash-native', 'gh1')`,
			refused: `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj4', 4, 'PaperPlane', 'Stash', 'gh1')`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := testDB(t)
			seed(t, pool)
			for _, sql := range tt.setup {
				mustExec(t, pool, sql)
			}
			if tt.allowed != "" {
				mustExec(t, pool, tt.allowed)
			}
			_, err := pool.Exec(context.Background(), tt.refused)
			if !isUniqueViolation(err) {
				t.Fatalf("the database accepted or mis-refused the row: err = %v, want a unique violation", err)
			}
		})
	}
}

func TestHandStatusesAreTheNineOfTheSpec(t *testing.T) {
	pool := testDB(t)
	seed(t, pool)
	// FR-QUEUE-2 lists the statuses; anything else is a bug in the caller.
	_, err := pool.Exec(context.Background(), `INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh2', 'Bad status', 'done')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("status 'done': err = %v, want a check violation", err)
	}
}

func TestNoTableStoresAnEmailAddress(t *testing.T) {
	pool := testDB(t)
	// NFR-PRIV-1: the app never stores email addresses.
	rows, err := pool.Query(context.Background(),
		`SELECT table_name || '.' || column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND column_name ILIKE '%mail%'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("NFR-PRIV-1: column %s looks like it stores an email address", col)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
