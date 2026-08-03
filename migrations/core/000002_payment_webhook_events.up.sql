CREATE TABLE payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(64) NOT NULL,
    event_id VARCHAR(255) NOT NULL,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider_reference VARCHAR(255),
    event_status VARCHAR(32) NOT NULL
        CHECK (event_status IN ('pending', 'paid', 'failed')),
    amount BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL,
    processing_status VARCHAR(32) NOT NULL DEFAULT 'processing'
        CHECK (processing_status IN ('processing', 'processed')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMPTZ,
    UNIQUE (provider, event_id)
);
CREATE INDEX payment_webhook_events_order_received_idx
    ON payment_webhook_events (order_id, received_at);
