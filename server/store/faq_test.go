package store

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// faqSeed adds what the FAQ tests read: two maintainers, two projects, and in proj1 six answers
// that a search for "cache" can tell apart by rank and by age, plus one that was deleted. proj2
// has one answer under a slug proj1 uses too. Nothing but the store's Done writes faq_entries, so
// the rows are written with SQL.
func faqSeed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO users (id, github_id, login, name, avatar_url) VALUES
		('gh1', 1, 'mia-k', 'Mia Kowalski', 'https://avatars.example/1'),
		('gh2', 2, 'diego-ruiz', '', '')`)
	mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES
		('proj1', 1, 'PaperPlane', 'Stash', 'gh1'),
		('proj2', 2, 'PaperPlane', 'Other', 'gh1')`)
	mustExec(t, pool, `INSERT INTO sessions (project_id, title) VALUES ('proj1', 'Thursday office hours')`)
	mustExec(t, pool, `INSERT INTO hands (session_id, author_id, title, status) VALUES (1, 'gh2', 'How do I clear the cache?', 'answered')`)
	mustExec(t, pool, `INSERT INTO faq_entries (project_id, hand_id, slug, title, question, answer, links, asked_by, author_id, created_at, deleted_at) VALUES
		('proj1', 1, 'clear-the-cache', 'How do I clear the cache?',
		 'I changed the key format and old entries keep coming back.', 'Call stash.clear() once after the upgrade.',
		 '["https://example.com/docs/clear", "https://example.com/changelog"]', 3, 'gh1', '2026-11-01 10:00+00', NULL),
		('proj1', NULL, 'where-is-the-cache', 'Where is the cache stored?',
		 '', 'On disk, under ~/.stash, unless you pass a directory.',
		 '[]', 1, 'gh2', '2026-11-03 10:00+00', NULL),
		('proj1', NULL, 'safe-for-concurrent-use', 'Is Stash safe for concurrent use?',
		 '', 'Yes. The cache locks per key, the cache index is copied on write, and the cache never blocks readers.',
		 '[]', 1, 'gh1', '2026-11-02 10:00+00', NULL),
		('proj1', NULL, 'retry-only-on-5xx', 'How do I retry only on 5xx errors?',
		 '', 'Use retryIf and check the status. The cache is not touched by a retry.',
		 '[]', 1, 'gh1', '2026-11-05 10:00+00', NULL),
		('proj1', NULL, 'set-a-timeout', 'How do I set a timeout?',
		 '', 'Pass a context with a deadline to every call.',
		 '[]', 1, 'gh1', '2026-11-06 10:00+00', NULL),
		('proj1', NULL, 'works-on-windows', 'Does it work on Windows?',
		 '', 'Yes, since version 2.',
		 '[]', 1, 'gh1', '2026-11-04 10:00+00', NULL),
		('proj1', NULL, 'old-cache-format', 'What was the old cache format?',
		 '', 'It is gone.',
		 '[]', 1, 'gh1', '2026-11-07 10:00+00', '2026-11-08 10:00+00'),
		('proj2', NULL, 'clear-the-cache', 'How do I clear the cache?',
		 '', 'The answer of another project.',
		 '[]', 1, 'gh1', '2026-11-08 10:00+00', NULL)`)
}

// faqSlugs lists the slugs of entries in the order they came.
func faqSlugs(entries []FAQEntry) []string {
	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		slugs = append(slugs, e.Slug)
	}
	return slugs
}

// faqNasty is text no search may choke on: quotes, a backslash, the operators of both query
// syntaxes, and an address whose lexemes carry quotes and ampersands. None of its words is in an
// answer.
const faqNasty = `'"; DROP TABLE users; -- ((( &&& !!! | <-> \ "unclosed http://example.com/it's?a=1&b='2' C:\dir`

func TestFAQList(t *testing.T) {
	pool := testDB(t)
	faqSeed(t, pool)
	s, ctx := New(pool), context.Background()

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{"FR-FAQ-3: without a query the newest come first, and the deleted one and other projects stay out", "", 50,
			[]string{"set-a-timeout", "retry-only-on-5xx", "works-on-windows", "where-is-the-cache", "safe-for-concurrent-use", "clear-the-cache"}},
		{"FR-FAQ-3: the limit is respected", "", 2,
			[]string{"set-a-timeout", "retry-only-on-5xx"}},
		{"FR-FAQ-3: a match in the title comes before one in the answer, more mentions before fewer, and of equals the newest", "cache", 50,
			[]string{"where-is-the-cache", "clear-the-cache", "safe-for-concurrent-use", "retry-only-on-5xx"}},
		{"FR-FAQ-3: the limit keeps the best matches", "cache", 1,
			[]string{"where-is-the-cache"}},
		{"FR-FAQ-3: another form of a word finds the same answers", "caching", 50,
			[]string{"where-is-the-cache", "clear-the-cache", "safe-for-concurrent-use", "retry-only-on-5xx"}},
		{"FR-FAQ-3: every word of the query must match", "cache disk", 50,
			[]string{"where-is-the-cache"}},
		{"FR-FAQ-3: the details of a question are searched, FR-FAQ-6: a deleted entry never matches", "format", 50,
			[]string{"clear-the-cache"}},
		{"FR-FAQ-3: the answer is searched", "deadline", 50,
			[]string{"set-a-timeout"}},
		{"FR-FAQ-3: a word nobody used finds nothing", "kubernetes", 50, []string{}},
		{"FR-FAQ-3: a query of small words only finds nothing", "how do I", 50, []string{}},
		{"NFR-SEC-3: text full of quotes and operators is a search like any other", faqNasty, 50, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.FAQList(ctx, "proj1", tt.query, tt.limit)
			if err != nil {
				t.Fatalf("FAQList(%q): %v", tt.query, err)
			}
			if !slices.Equal(faqSlugs(got), tt.want) {
				t.Fatalf("FAQList(%q, limit %d) = %v, want %v", tt.query, tt.limit, faqSlugs(got), tt.want)
			}
		})
	}

	t.Run("FR-FAQ-3: a project without answers has an empty list", func(t *testing.T) {
		mustExec(t, pool, `INSERT INTO projects (id, github_id, owner, repo, created_by) VALUES ('proj3', 3, 'PaperPlane', 'Empty', 'gh1')`)
		for _, query := range []string{"", "cache"} {
			if got, err := s.FAQList(ctx, "proj3", query, 50); err != nil || len(got) != 0 {
				t.Errorf("FAQList(proj3, %q) = %v, %v; want nothing", query, faqSlugs(got), err)
			}
		}
	})
}

func TestFAQEntriesComeWithTheirPeopleAndLinks(t *testing.T) {
	pool := testDB(t)
	faqSeed(t, pool)
	s, ctx := New(pool), context.Background()

	list, err := s.FAQList(ctx, "proj1", "", 50)
	if err != nil {
		t.Fatalf("FAQList: %v", err)
	}
	bySlug := map[string]FAQEntry{}
	for _, e := range list {
		bySlug[e.Slug] = e
	}

	t.Run("FR-FAQ-3: an entry carries its text, its links, its date and the maintainer who answered", func(t *testing.T) {
		e := bySlug["clear-the-cache"]
		if e.ID == 0 || e.ProjectID != "proj1" || e.HandID != 1 || e.Title != "How do I clear the cache?" ||
			e.Question != "I changed the key format and old entries keep coming back." ||
			e.Answer != "Call stash.clear() once after the upgrade." || e.Deleted {
			t.Errorf("entry = %+v", e)
		}
		if !slices.Equal(e.Links, []string{"https://example.com/docs/clear", "https://example.com/changelog"}) {
			t.Errorf("links = %v, want the two of the JSON column in their order", e.Links)
		}
		if want := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC); !e.CreatedAt.Equal(want) {
			t.Errorf("created at %v, want %v", e.CreatedAt, want)
		}
		if want := (Person{UserID: "gh1", Login: "mia-k", Name: "Mia Kowalski", AvatarURL: "https://avatars.example/1"}); e.Author != want {
			t.Errorf("author = %+v, want %+v", e.Author, want)
		}
	})

	t.Run("FR-HAND-8: how many people asked is a number", func(t *testing.T) {
		if got := bySlug["clear-the-cache"].AskedBy; got != 3 {
			t.Errorf("asked by %d, want 3", got)
		}
		if got := bySlug["set-a-timeout"].AskedBy; got != 1 {
			t.Errorf("asked by %d, want the column's default of 1", got)
		}
	})

	t.Run("FR-FAQ-3: an entry without a hand or links, answered by someone without a display name", func(t *testing.T) {
		e := bySlug["where-is-the-cache"]
		if e.HandID != 0 || len(e.Links) != 0 || e.Question != "" {
			t.Errorf("entry = %+v, want no hand, no links and no details", e)
		}
		if want := (Person{UserID: "gh2", Login: "diego-ruiz", Name: "diego-ruiz"}); e.Author != want {
			t.Errorf("author = %+v, want the login standing in for the name: %+v", e.Author, want)
		}
	})

	t.Run("FR-FAQ-3, FR-FAQ-4: a search and a suggestion carry the same", func(t *testing.T) {
		found, err := s.FAQList(ctx, "proj1", "upgrade", 50)
		if err != nil || len(found) != 1 {
			t.Fatalf("FAQList(upgrade) = %v, %v; want one entry", faqSlugs(found), err)
		}
		suggested, err := s.FAQSuggest(ctx, "proj1", "what do I call after the upgrade?", 3)
		if err != nil || len(suggested) == 0 {
			t.Fatalf("FAQSuggest = %v, %v; want an entry", faqSlugs(suggested), err)
		}
		want := bySlug["clear-the-cache"]
		for name, got := range map[string]FAQEntry{"search": found[0], "suggestion": suggested[0]} {
			if got.ID != want.ID || got.Author != want.Author || got.AskedBy != 3 || !slices.Equal(got.Links, want.Links) || !got.CreatedAt.Equal(want.CreatedAt) {
				t.Errorf("%s = %+v, want %+v", name, got, want)
			}
		}
	})
}

func TestFAQBySlug(t *testing.T) {
	pool := testDB(t)
	faqSeed(t, pool)
	s, ctx := New(pool), context.Background()

	t.Run("FR-FAQ-3: an entry is found by its slug inside its own project", func(t *testing.T) {
		mine, found, err := s.FAQBySlug(ctx, "proj1", "clear-the-cache")
		if err != nil || !found || mine.ProjectID != "proj1" || mine.Answer != "Call stash.clear() once after the upgrade." || mine.Author.Login != "mia-k" || mine.Deleted {
			t.Fatalf("FAQBySlug(proj1) = %+v, %v, %v", mine, found, err)
		}
		theirs, found, err := s.FAQBySlug(ctx, "proj2", "clear-the-cache")
		if err != nil || !found || theirs.ProjectID != "proj2" || theirs.Answer != "The answer of another project." || theirs.ID == mine.ID {
			t.Fatalf("FAQBySlug(proj2) = %+v, %v, %v; want the other project's own entry", theirs, found, err)
		}
	})

	t.Run("FR-FAQ-6: a deleted entry is found, and says that it is deleted", func(t *testing.T) {
		e, found, err := s.FAQBySlug(ctx, "proj1", "old-cache-format")
		if err != nil || !found || !e.Deleted || e.Title != "What was the old cache format?" {
			t.Fatalf("FAQBySlug(deleted) = %+v, %v, %v; want it found with Deleted set", e, found, err)
		}
	})

	t.Run("FR-FAQ-3: an unknown slug, or one of another project, is not found and is no error", func(t *testing.T) {
		for _, c := range []struct{ project, slug string }{
			{"proj1", "no-such-answer"}, {"proj2", "set-a-timeout"}, {"no-such-project", "clear-the-cache"}, {"proj1", ""},
		} {
			if e, found, err := s.FAQBySlug(ctx, c.project, c.slug); err != nil || found || e.ID != 0 {
				t.Errorf("FAQBySlug(%q, %q) = %+v, %v, %v; want not found", c.project, c.slug, e, found, err)
			}
		}
	})
}

func TestFAQSuggest(t *testing.T) {
	pool := testDB(t)
	faqSeed(t, pool)
	s, ctx := New(pool), context.Background()

	tests := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{"FR-FAQ-4: a sentence that shares two words with one answer finds it, and none of the answers it shares no word with",
			"My retry loop also fires on 404 errors, why?", 3,
			[]string{"retry-only-on-5xx"}},
		{"FR-FAQ-4: any shared word is enough, the answer that shares the most comes first, and the limit holds",
			"Where does the cache live on disk, and can I clear it?", 3,
			[]string{"clear-the-cache", "where-is-the-cache", "safe-for-concurrent-use"}},
		{"FR-FAQ-4: with room for all of them, the weakest match is last",
			"Where does the cache live on disk, and can I clear it?", 10,
			[]string{"clear-the-cache", "where-is-the-cache", "safe-for-concurrent-use", "retry-only-on-5xx"}},
		{"FR-FAQ-6: a deleted entry is never suggested",
			"what was the old format", 3,
			[]string{"clear-the-cache"}},
		{"FR-FAQ-4: a text with no usable word suggests nothing", "how do I do it?", 3, []string{}},
		{"FR-FAQ-4: an empty text suggests nothing", "", 3, []string{}},
		{"FR-FAQ-4: punctuation suggests nothing", "??? !!! ...", 3, []string{}},
		{"FR-FAQ-4: words nobody used suggest nothing", "kubernetes operator helm chart", 3, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.FAQSuggest(ctx, "proj1", tt.text, tt.limit)
			if err != nil {
				t.Fatalf("FAQSuggest(%q): %v", tt.text, err)
			}
			if !slices.Equal(faqSlugs(got), tt.want) {
				t.Fatalf("FAQSuggest(%q, limit %d) = %v, want %v", tt.text, tt.limit, faqSlugs(got), tt.want)
			}
		})
	}

	t.Run("NFR-SEC-3: a draft full of quotes, backslashes and operators is text, and its plain words still count", func(t *testing.T) {
		got, err := s.FAQSuggest(ctx, "proj1", faqNasty+" timeout", 3)
		if err != nil {
			t.Fatalf("FAQSuggest: %v", err)
		}
		if !slices.Equal(faqSlugs(got), []string{"set-a-timeout"}) {
			t.Fatalf("FAQSuggest = %v, want the one answer about a timeout", faqSlugs(got))
		}
		if n := count(t, pool, `SELECT count(*) FROM faq_entries`); n != 8 {
			t.Fatalf("%d entries after the search, want the 8 that were there", n)
		}
	})

	t.Run("FR-FAQ-4: a draft as long as the form allows is searched", func(t *testing.T) {
		long := strings.Repeat("something unrelated and wordy ", 70) + "windows"
		got, err := s.FAQSuggest(ctx, "proj1", long, 3)
		if err != nil || !slices.Equal(faqSlugs(got), []string{"works-on-windows"}) {
			t.Fatalf("FAQSuggest(%d characters) = %v, %v; want the answer about Windows", len(long), faqSlugs(got), err)
		}
	})

	t.Run("FR-FAQ-4: suggestions stay inside the project", func(t *testing.T) {
		got, err := s.FAQSuggest(ctx, "proj2", "how do I clear the cache", 3)
		if err != nil || len(got) != 1 || got[0].ProjectID != "proj2" {
			t.Fatalf("FAQSuggest(proj2) = %+v, %v; want its own one entry", got, err)
		}
	})
}

func TestDeleteFAQ(t *testing.T) {
	pool := testDB(t)
	faqSeed(t, pool)
	s, ctx := New(pool), context.Background()

	entry, found, err := s.FAQBySlug(ctx, "proj1", "set-a-timeout")
	if err != nil || !found {
		t.Fatalf("FAQBySlug: %v, %v", found, err)
	}
	deletedAt := func() *time.Time {
		t.Helper()
		var at *time.Time
		if err := pool.QueryRow(ctx, `SELECT deleted_at FROM faq_entries WHERE id = $1`, entry.ID).Scan(&at); err != nil {
			t.Fatalf("read deleted_at: %v", err)
		}
		return at
	}

	t.Run("FR-FAQ-6: ProjectOfFAQ names the entry's project, and knows no entry it was not given", func(t *testing.T) {
		if projectID, found, err := s.ProjectOfFAQ(ctx, entry.ID); err != nil || !found || projectID != "proj1" {
			t.Errorf("ProjectOfFAQ = %q, %v, %v; want proj1", projectID, found, err)
		}
		if projectID, found, err := s.ProjectOfFAQ(ctx, 999999); err != nil || found || projectID != "" {
			t.Errorf("ProjectOfFAQ(unknown) = %q, %v, %v; want not found", projectID, found, err)
		}
	})

	t.Run("FR-FAQ-6: a deleted entry leaves every list, and is still found by its slug", func(t *testing.T) {
		if err := s.DeleteFAQ(ctx, entry.ID); err != nil {
			t.Fatalf("DeleteFAQ: %v", err)
		}
		if deletedAt() == nil {
			t.Fatal("deleted_at is not set after DeleteFAQ")
		}
		list, err := s.FAQList(ctx, "proj1", "", 50)
		if err != nil || slices.Contains(faqSlugs(list), "set-a-timeout") || len(list) != 5 {
			t.Errorf("FAQList = %v, %v; want the five that are left", faqSlugs(list), err)
		}
		if found, err := s.FAQList(ctx, "proj1", "timeout", 50); err != nil || len(found) != 0 {
			t.Errorf("FAQList(timeout) = %v, %v; want nothing", faqSlugs(found), err)
		}
		if suggested, err := s.FAQSuggest(ctx, "proj1", "how do I set a timeout", 3); err != nil || len(suggested) != 0 {
			t.Errorf("FAQSuggest = %v, %v; want nothing", faqSlugs(suggested), err)
		}
		gone, found, err := s.FAQBySlug(ctx, "proj1", "set-a-timeout")
		if err != nil || !found || !gone.Deleted || gone.ID != entry.ID {
			t.Errorf("FAQBySlug = %+v, %v, %v; want the entry with Deleted set", gone, found, err)
		}
	})

	t.Run("FR-FAQ-6: deleting twice is fine, and keeps the time of the first", func(t *testing.T) {
		first := deletedAt()
		if err := s.DeleteFAQ(ctx, entry.ID); err != nil {
			t.Fatalf("second DeleteFAQ: %v", err)
		}
		if second := deletedAt(); first == nil || second == nil || !second.Equal(*first) {
			t.Errorf("deleted_at went from %v to %v, want it unchanged", first, second)
		}
		// Whoever deletes still has to be told whose entry it is (the route checks their role).
		if projectID, found, err := s.ProjectOfFAQ(ctx, entry.ID); err != nil || !found || projectID != "proj1" {
			t.Errorf("ProjectOfFAQ(deleted) = %q, %v, %v; want proj1", projectID, found, err)
		}
	})

	t.Run("FR-FAQ-6: deleting an entry that never existed changes nothing and is no error", func(t *testing.T) {
		if err := s.DeleteFAQ(ctx, 999999); err != nil {
			t.Fatalf("DeleteFAQ(unknown): %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM faq_entries WHERE deleted_at IS NULL`); n != 6 {
			t.Errorf("%d entries are left, want 6", n)
		}
	})
}
