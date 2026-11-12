-- Office Hours, first schema (docs/ARCHITECTURE.md §3). Rules that racing requests must not break
-- are unique indexes here, so the database refuses the second writer.

CREATE TABLE users (
  id         TEXT PRIMARY KEY,                -- 'gh' || github_id; also the Nexconn user id (FR-AUTH-2)
  github_id  BIGINT NOT NULL UNIQUE,
  login      TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ                      -- account deletion blanks the row and keeps the id (FR-AUTH-5)
);                                            -- NFR-PRIV-1: no email address, here or anywhere

CREATE TABLE projects (
  id               TEXT PRIMARY KEY,          -- random: it names the Nexconn group, so it must not be guessable
  owner            TEXT NOT NULL,
  repo             TEXT NOT NULL,
  description      TEXT NOT NULL DEFAULT '',
  nexconn_group_id TEXT NOT NULL UNIQUE,
  next_session_at  TIMESTAMPTZ,
  created_by       TEXT NOT NULL REFERENCES users (id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- FR-PROJ-2: one project per repository, whatever the letter case.
CREATE UNIQUE INDEX projects_one_per_repo ON projects (lower(owner), lower(repo));

-- The source of truth for roles (NFR-SEC-2).
CREATE TABLE maintainers (
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  user_id    TEXT NOT NULL REFERENCES users (id),
  PRIMARY KEY (project_id, user_id)
);

-- Written by join; also the "unique participants" count (FR-MET-1).
CREATE TABLE members (
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  user_id    TEXT NOT NULL REFERENCES users (id),
  joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, user_id)
);
CREATE INDEX members_by_user ON members (user_id);

CREATE TABLE sessions (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  title           TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'ended')),
  planned_minutes INT,
  started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at        TIMESTAMPTZ,
  hands_closed    BOOLEAN NOT NULL DEFAULT false,
  queue_version   INT NOT NULL DEFAULT 0
);
-- FR-SESS-2: one live session per project.
CREATE UNIQUE INDEX sessions_one_live_per_project ON sessions (project_id) WHERE status = 'live';

CREATE TABLE hands (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_id  BIGINT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
  author_id   TEXT NOT NULL REFERENCES users (id),
  title       TEXT NOT NULL,
  details     TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN (
                'waiting', 'answering', 'answered', 'skipped', 'withdrawn',
                'removed', 'not_reached', 'carried', 'merged')),  -- FR-QUEUE-2
  raised_at   TIMESTAMPTZ NOT NULL DEFAULT now(),  -- never changes
  sort_at     TIMESTAMPTZ NOT NULL DEFAULT now(),  -- the queue's order; moves on Away
  taken_at    TIMESTAMPTZ,
  closed_at   TIMESTAMPTZ,
  acked_at    TIMESTAMPTZ,                         -- "I'm here" (FR-QUEUE-9)
  away_count  INT NOT NULL DEFAULT 0,
  merged_into BIGINT REFERENCES hands (id)
);
-- FR-HAND-2: one open hand per user per session.
CREATE UNIQUE INDEX hands_one_open_per_author ON hands (session_id, author_id) WHERE status IN ('waiting', 'answering');
-- FR-QUEUE-3: one hand answered at a time.
CREATE UNIQUE INDEX hands_one_answering_per_session ON hands (session_id) WHERE status = 'answering';

-- "+1 me too" (FR-HAND-7). It never changes the order.
CREATE TABLE hand_plus (
  hand_id BIGINT NOT NULL REFERENCES hands (id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users (id),
  PRIMARY KEY (hand_id, user_id)
);

CREATE TABLE faq_entries (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  hand_id    BIGINT REFERENCES hands (id) ON DELETE SET NULL,
  slug       TEXT NOT NULL,
  title      TEXT NOT NULL,
  question   TEXT NOT NULL DEFAULT '',
  answer     TEXT NOT NULL,
  links      JSONB NOT NULL DEFAULT '[]',
  author_id  TEXT NOT NULL REFERENCES users (id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,                          -- a deleted entry answers 410 (FR-FAQ-6)
  search     tsvector GENERATED ALWAYS AS (
               to_tsvector('english', title || ' ' || question || ' ' || answer)) STORED,
  UNIQUE (project_id, slug)                        -- FR-FAQ-1
);
CREATE INDEX faq_entries_search ON faq_entries USING GIN (search);  -- FR-FAQ-4
