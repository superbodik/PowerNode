-- Song requests via Twitch Channel Points, as an alternative to the
-- Spotify-based flow (0025): a viewer redeems a custom "Song Request"
-- reward this panel creates on the broadcaster's channel, pastes a
-- YouTube/SoundCloud/Yandex Music link as the reward's required text
-- input, and it lands in a queue an OBS Browser Source plays through --
-- no Spotify Premium/allowlist involved at all, just Channel Points.
--
-- song_request_rewards: one row per streamer who's turned this on. Reuses
-- the twitch_eventsub_subscriptions table (0018) for the underlying
-- channel.channel_points_custom_reward_redemption.add subscription --
-- render_token here is specifically the OBS widget's auth, same
-- possession-token model as twitch_widget_tokens, kept separate so
-- sharing this URL (which ends up in an OBS scene) can't do anything to
-- the sub/follow alert widget or vice versa.
CREATE TABLE song_request_rewards (
    user_id           BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    twitch_reward_id  TEXT NOT NULL,
    render_token      TEXT NOT NULL UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per redemption. provider is classified from the link at
-- ingestion time so the panel's queue view can show an icon without
-- re-parsing the URL; the widget re-derives the actual embed src itself
-- since that's dependent on exactly how each provider's embed player wants
-- the URL shaped.
CREATE TABLE song_request_queue (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    twitch_redemption_id  TEXT NOT NULL,
    redeemer_name         TEXT NOT NULL,
    link                  TEXT NOT NULL,
    provider              TEXT NOT NULL DEFAULT 'unknown',
    status                TEXT NOT NULL DEFAULT 'queued',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, twitch_redemption_id)
);
CREATE INDEX idx_song_request_queue_user_status ON song_request_queue(user_id, status, id);
