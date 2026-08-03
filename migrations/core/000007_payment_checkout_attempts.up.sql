-- Durable recovery state for the interval between creating a pending order and
-- recording the provider payment reference. Ephemeral redirect/client-secret
-- data is deliberately never stored here.
CREATE TABLE payment_checkout_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    amount BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL,
    provider_reference VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'created', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX payment_checkout_attempts_recovery_idx
    ON payment_checkout_attempts(status, updated_at)
    WHERE status = 'creating';
