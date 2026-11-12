package store

import "context"

// Owned by the FAQ-pages worker of Phase 3 (notes/phase-2-plan.md, worker 7). The signatures are
// a contract; the bodies are theirs.

// FAQList lists a project's entries, the newest first, without the deleted ones. With a query it
// lists the entries that match it in full-text search, the best match first (FR-FAQ-3).
func (s *Store) FAQList(ctx context.Context, projectID, query string, limit int) ([]FAQEntry, error) {
	return nil, ErrNotBuilt
}

// FAQBySlug finds one entry. A deleted entry is found too, with Deleted set (FR-FAQ-6).
func (s *Store) FAQBySlug(ctx context.Context, projectID, slug string) (FAQEntry, bool, error) {
	return FAQEntry{}, false, ErrNotBuilt
}

// FAQSuggest finds up to limit entries similar to a question someone is typing (FR-FAQ-4).
func (s *Store) FAQSuggest(ctx context.Context, projectID, text string, limit int) ([]FAQEntry, error) {
	return nil, ErrNotBuilt
}

// ProjectOfFAQ says which project an entry belongs to.
func (s *Store) ProjectOfFAQ(ctx context.Context, id int64) (projectID string, found bool, err error) {
	return "", false, ErrNotBuilt
}

// DeleteFAQ marks an entry deleted; its page answers 410 from then on (FR-FAQ-6).
func (s *Store) DeleteFAQ(ctx context.Context, id int64) error {
	return ErrNotBuilt
}
