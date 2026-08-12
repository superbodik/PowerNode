-- Donations via Stripe Connect: non-custodial by design -- each streamer
-- connects their own Stripe Express account (Stripe hosts the entire
-- onboarding/KYC flow, we never see or store bank/identity details) and
-- funds go straight there via Checkout's transfer_data.destination, minus
-- a platform fee (application_fee_amount). We never hold donor funds
-- ourselves, which keeps this out of money-transmitter territory.
CREATE TABLE stripe_connections (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    stripe_account_id TEXT NOT NULL UNIQUE,
    charges_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    payouts_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    details_submitted BOOLEAN NOT NULL DEFAULT FALSE,
    connected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per completed donation, written from the checkout.session.completed
-- webhook (the source of truth -- nothing is recorded just from a viewer
-- reaching the checkout page, only once Stripe confirms payment).
CREATE TABLE donations (
    id                     BIGSERIAL PRIMARY KEY,
    recipient_user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stripe_checkout_session_id TEXT NOT NULL UNIQUE,
    stripe_payment_intent_id  TEXT,
    donor_name             TEXT NOT NULL DEFAULT '',
    message                TEXT NOT NULL DEFAULT '',
    amount_cents           BIGINT NOT NULL,
    currency               TEXT NOT NULL,
    platform_fee_cents     BIGINT NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_donations_recipient ON donations(recipient_user_id, created_at DESC);
