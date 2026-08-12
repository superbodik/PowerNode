-- First-pass Twitch integration: connect an account and remember who's who.
-- Deliberately minimal scope (user:read:email only) -- this just proves out
-- identity and the OAuth plumbing. Stream title/category management and a
-- sub/donation alert widget are follow-up work once this exists to build on,
-- not part of this migration.

-- Short-lived CSRF state for the OAuth redirect round trip. The browser
-- leaves this app entirely (navigates to twitch.tv and back), so the
-- initiating user can't be recovered from a normal Authorization header on
-- callback -- state is how the callback knows which PowerNode user this
-- belongs to. Rows are deleted on use (or opportunistically once stale) by
-- the handler, not by a cron job.
CREATE TABLE twitch_oauth_states (
    state       TEXT PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE twitch_connections (
    user_id                 BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    twitch_user_id          TEXT NOT NULL,
    twitch_login            TEXT NOT NULL,
    access_token_encrypted  TEXT NOT NULL,
    refresh_token_encrypted TEXT NOT NULL,
    scopes                  TEXT NOT NULL,
    connected_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
