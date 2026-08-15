CREATE TABLE stream_sessions (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    started_at     TIMESTAMPTZ NOT NULL,
    ended_at       TIMESTAMPTZ,
    peak_viewers   INT NOT NULL DEFAULT 0,
    chat_messages  INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_stream_sessions_user_started ON stream_sessions(user_id, started_at DESC);

CREATE TABLE viewer_samples (
    id          BIGSERIAL PRIMARY KEY,
    session_id  BIGINT NOT NULL REFERENCES stream_sessions(id) ON DELETE CASCADE,
    sampled_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    viewers     INT NOT NULL
);
CREATE INDEX idx_viewer_samples_session ON viewer_samples(session_id, sampled_at);
