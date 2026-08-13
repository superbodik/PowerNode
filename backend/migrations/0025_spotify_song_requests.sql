-- Song requests via donations: a streamer connects Spotify (their own
-- account, same non-custodial shape as Stripe -- we never touch their
-- library, only queue tracks), a donor optionally names a track, and the
-- webhook looks it up and adds it to the streamer's live playback queue.
CREATE TABLE spotify_oauth_states (
    state      TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE spotify_connections (
    id                      BIGSERIAL PRIMARY KEY,
    user_id                 BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    spotify_user_id         TEXT NOT NULL,
    display_name            TEXT NOT NULL DEFAULT '',
    access_token_encrypted  TEXT NOT NULL,
    refresh_token_encrypted TEXT NOT NULL,
    connected_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Recorded regardless of whether the live queue action actually succeeded
-- (streamer's Spotify closed, no active device, track not found) -- so
-- there's still a record of what was requested even when the on-stream
-- side effect silently no-ops.
ALTER TABLE donations ADD COLUMN IF NOT EXISTS track_query TEXT NOT NULL DEFAULT '';
ALTER TABLE donations ADD COLUMN IF NOT EXISTS track_queued BOOLEAN NOT NULL DEFAULT FALSE;
