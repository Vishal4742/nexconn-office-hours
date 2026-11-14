-- FR-MOD-1, AT-18: when a member's mute in the room ends. Nexconn mutes them in the chat, but a
-- raised hand reaches the room through the bot, so the server keeps its own record to refuse one.
-- NULL: never muted. Adding a nullable column leaves the code already running as it is.
ALTER TABLE members ADD COLUMN IF NOT EXISTS muted_until TIMESTAMPTZ;
