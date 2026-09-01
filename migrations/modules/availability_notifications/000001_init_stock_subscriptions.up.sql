-- Availability notifications owns customer opt-ins. Variant IDs are logical
-- Catalog identities, deliberately without cross-module foreign keys.
CREATE TABLE stock_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL,
    email VARCHAR(320) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notified_at TIMESTAMPTZ,
    CHECK (status IN ('pending', 'notified', 'unsubscribed')),
    CHECK (email <> '')
);

CREATE UNIQUE INDEX stock_subscriptions_pending_variant_email_unique
    ON stock_subscriptions (variant_id, email) WHERE status = 'pending';
CREATE INDEX stock_subscriptions_pending_variant_idx
    ON stock_subscriptions (variant_id, created_at, id) WHERE status = 'pending';
