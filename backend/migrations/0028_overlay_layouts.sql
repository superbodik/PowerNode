-- Moderator overlay constructor: a streamer (or anyone holding the
-- moderator link -- token-based access, no login, same trust model as the
-- alert widget) arranges widgets (chat, viewer count, donation total,
-- text, image) on a canvas, and a separate public render page turns that
-- layout into the actual OBS Browser Source.
--
-- Position/size are stored as percentages (0-100) of the canvas, not
-- pixels -- this is what makes the same layout render correctly at
-- whatever resolution OBS's Browser Source actually is, without needing
-- to know it up front.
CREATE TABLE overlay_layouts (
    id              BIGSERIAL PRIMARY KEY,
    owner_user_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL DEFAULT 'Overlay',
    -- Grants edit access to the constructor -- anyone with this link, not
    -- just the owner, same shape as twitch_widget_tokens: possession of
    -- the token is the access control, deliberately, so a streamer can
    -- hand it to a moderator without creating them a panel account.
    moderator_token TEXT NOT NULL UNIQUE,
    -- Grants read-only access to the public render page (the actual OBS
    -- Browser Source URL) -- separate from moderator_token so sharing the
    -- render URL (which might end up visible on stream, screenshots, etc.)
    -- never leaks edit access.
    render_token    TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE overlay_widgets (
    id          BIGSERIAL PRIMARY KEY,
    layout_id   BIGINT NOT NULL REFERENCES overlay_layouts(id) ON DELETE CASCADE,
    widget_type TEXT NOT NULL, -- 'chat', 'viewer_count', 'donation_total', 'text', 'image'
    x           DOUBLE PRECISION NOT NULL DEFAULT 5,
    y           DOUBLE PRECISION NOT NULL DEFAULT 5,
    width       DOUBLE PRECISION NOT NULL DEFAULT 30,
    height      DOUBLE PRECISION NOT NULL DEFAULT 20,
    z_index     INT NOT NULL DEFAULT 0,
    -- Widget-specific settings (text content/font size for 'text', image
    -- URL for 'image', label overrides for the data-driven ones) -- a
    -- JSONB blob rather than a column per widget type since the shape
    -- genuinely differs per type and there's no shared query need to
    -- filter on any of these fields.
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_overlay_widgets_layout ON overlay_widgets(layout_id);
