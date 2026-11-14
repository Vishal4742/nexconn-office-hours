-- Agents a person connected through the MCP sign-in (FR-AGENT-2, FR-AGENT-3): one row per
-- approval. It holds no token. The access token is sealed with AUTH_SECRET and kept by the agent's
-- app, and every request checks its row here, so Disconnect (revoked_at) ends it at once.

CREATE TABLE agent_grants (
  id            TEXT PRIMARY KEY,                     -- random, named inside the sealed token: an
                                                      -- authorization code turned in twice hits it
  user_id       TEXT NOT NULL REFERENCES users (id),
  client_name   TEXT NOT NULL,                        -- what the app called itself; shown as text
  redirect_host TEXT NOT NULL,                        -- where the consent page sent the person back
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at  TIMESTAMPTZ,                          -- noted at most once a minute
  revoked_at    TIMESTAMPTZ                           -- Disconnect, or a code that came back twice
);

CREATE INDEX agent_grants_by_user ON agent_grants (user_id) WHERE revoked_at IS NULL;
