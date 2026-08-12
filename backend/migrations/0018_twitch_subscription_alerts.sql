-- Second pass on the Twitch integration: real-time subscription alerts for
-- an OBS Browser Source, on top of the account connection from 0017.
--
-- twitch_widget_tokens: the opaque secret embedded in the OBS-facing widget
-- URL. OBS Browser Source can't do an OAuth/login flow -- the token in the
-- URL *is* the auth, same idea as an API key.
--
-- twitch_eventsub_subscriptions: tracks what we've registered with Twitch's
-- EventSub so a repeat "enable" call doesn't double-register, and so a
-- disconnect can clean them up. twitch_subscription_id is Twitch's own ID
-- for the subscription (returned from the create call), not ours.
CREATE TABLE twitch_widget_tokens (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE twitch_eventsub_subscriptions (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type            TEXT NOT NULL,
    twitch_subscription_id TEXT NOT NULL UNIQUE,
    status                TEXT NOT NULL DEFAULT 'enabled',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, event_type)
);
