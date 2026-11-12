-- Indexes for what the queue reads on every change (found in the phase-close review). Each
-- mutation pings the room, and every page in it reads the queue again: without these, each read
-- scans every hand and every FAQ entry ever written. IF NOT EXISTS, so running it twice is fine.

CREATE INDEX IF NOT EXISTS hands_by_session ON hands (session_id);
CREATE INDEX IF NOT EXISTS faq_entries_by_hand ON faq_entries (hand_id);
CREATE INDEX IF NOT EXISTS sessions_by_project ON sessions (project_id);
