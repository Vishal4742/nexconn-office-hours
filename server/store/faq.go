package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The FAQ's reads and its one write, the delete. An entry is created by Done (queue.go), in the
// same transaction that finishes its hand.

// faqSelect reads entries as faqScan wants them, each with the maintainer who answered it. Add a
// WHERE on f.
var faqSelect = `
	SELECT f.id, f.project_id, COALESCE(f.hand_id, 0), f.slug, f.title, f.question, f.answer, f.links,
	       f.asked_by, f.created_at, f.deleted_at IS NOT NULL, ` + personCols("author") + `
	FROM faq_entries f JOIN users author ON author.id = f.author_id`

// faqMatch is the rest of a search: the entries of project $1 that match the tsquery q and are
// not deleted, the best match first, then the newest, at most $2.
//
// The search column gives every word the same weight, so the title is counted once more with the
// highest: an answer whose question names the word comes before one that mentions it in passing.
const faqMatch = `
	WHERE f.project_id = $1 AND f.deleted_at IS NULL AND f.search @@ q
	ORDER BY ts_rank(setweight(to_tsvector('english', f.title), 'A') || f.search, q) DESC, f.created_at DESC, f.id DESC
	LIMIT $2`

// faqSearchable is typed search text as Postgres takes it: a NUL and the bytes that are no UTF-8
// are left out, because Postgres refuses a text that holds either. Search text is never stored,
// so nothing is lost.
func faqSearchable(text string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(text, ""), "\x00", "")
}

// faqScan reads one row of faqSelect.
func faqScan(row pgx.CollectableRow) (FAQEntry, error) {
	var e FAQEntry
	err := row.Scan(&e.ID, &e.ProjectID, &e.HandID, &e.Slug, &e.Title, &e.Question, &e.Answer, &e.Links,
		&e.AskedBy, &e.CreatedAt, &e.Deleted, &e.Author.UserID, &e.Author.Login, &e.Author.Name, &e.Author.AvatarURL)
	return e, err
}

// faqRows runs a faqSelect query and reads every entry it finds. what names the query in an error.
func (s *Store) faqRows(ctx context.Context, what, sql string, args ...any) ([]FAQEntry, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	entries, err := pgx.CollectRows(rows, faqScan)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	return entries, nil
}

// FAQList lists a project's entries, the newest first, without the deleted ones. With a query it
// lists the entries that match it in full-text search, the best match first (FR-FAQ-3). The query
// is what someone typed into a search field: every word must match, "quoted words" match as a
// phrase, and nothing in it can be a syntax error.
func (s *Store) FAQList(ctx context.Context, projectID, query string, limit int) ([]FAQEntry, error) {
	query = faqSearchable(query)
	if query == "" {
		return s.faqRows(ctx, "list FAQ entries", faqSelect+`
			WHERE f.project_id = $1 AND f.deleted_at IS NULL
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2`, projectID, limit)
	}
	return s.faqRows(ctx, "search FAQ entries", faqSelect+`
		CROSS JOIN websearch_to_tsquery('english', $3) AS q`+faqMatch, projectID, limit, query)
}

// FAQBySlug finds one entry. A deleted entry is found too, with Deleted set (FR-FAQ-6).
func (s *Store) FAQBySlug(ctx context.Context, projectID, slug string) (FAQEntry, bool, error) {
	entries, err := s.faqRows(ctx, "find FAQ entry", faqSelect+` WHERE f.project_id = $1 AND f.slug = $2`, projectID, slug)
	if err != nil || len(entries) == 0 {
		return FAQEntry{}, false, err
	}
	return entries[0], true, nil
}

// FAQSuggest finds up to limit entries similar to a question someone is typing (FR-FAQ-4), the
// most similar first. A draft is a sentence, and no answer holds all of a sentence's words, so an
// entry matches when it shares any of them: the query is the text's own lexemes joined with "|".
// Each lexeme is quoted, with its quotes and backslashes doubled, so none can be read as an
// operator. A text with no usable word (small words only, or nothing) has no lexemes: the query
// is NULL and nothing matches.
func (s *Store) FAQSuggest(ctx context.Context, projectID, text string, limit int) ([]FAQEntry, error) {
	return s.faqRows(ctx, "suggest FAQ entries", faqSelect+`
		CROSS JOIN (
			SELECT string_agg('''' || replace(replace(lexeme, E'\\', E'\\\\'), '''', '''''') || '''', ' | ')::tsquery
			FROM unnest(to_tsvector('english', $3::text))
		) AS words (q)`+faqMatch, projectID, limit, faqSearchable(text))
}

// ProjectOfFAQ says which project an entry belongs to. A deleted entry still belongs to its
// project: deleting it again has to find out whose it is.
func (s *Store) ProjectOfFAQ(ctx context.Context, id int64) (projectID string, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT project_id FROM faq_entries WHERE id = $1`, id).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: project of FAQ entry: %w", err)
	}
	return projectID, true, nil
}

// DeleteFAQ marks an entry deleted; its page answers 410 from then on (FR-FAQ-6). Deleting it
// again changes nothing, and neither does an id that names no entry.
func (s *Store) DeleteFAQ(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE faq_entries SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("store: delete FAQ entry: %w", err)
	}
	return nil
}
