-- What the queue needs to remember beyond the first schema (notes/phase-2-plan.md). IF NOT EXISTS,
-- because a development database may have been given these columns by hand before this file existed.

ALTER TABLE sessions    ADD COLUMN IF NOT EXISTS started_by TEXT REFERENCES users (id);  -- the maintainer who hosts it
ALTER TABLE hands       ADD COLUMN IF NOT EXISTS taken_by   TEXT REFERENCES users (id);  -- the maintainer who pressed Next
ALTER TABLE faq_entries ADD COLUMN IF NOT EXISTS asked_by   INT NOT NULL DEFAULT 1;      -- how many people asked: the author, the merged and the +1s (FR-HAND-8)
