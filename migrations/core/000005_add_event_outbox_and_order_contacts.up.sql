-- Core owns durable domain-event delivery because producers and consumers can
-- be optional modules. Payload is a versioned transport envelope, never
-- localized business content or an authoritative projection.
CREATE TABLE domain_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic VARCHAR(128) NOT NULL,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id UUID NOT NULL,
    idempotency_key UUID NOT NULL,
    payload TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (topic, idempotency_key)
);

CREATE INDEX domain_events_aggregate_idx ON domain_events (aggregate_type, aggregate_id, occurred_at);

CREATE TABLE event_deliveries (
    event_id UUID NOT NULL REFERENCES domain_events(id) ON DELETE CASCADE,
    consumer VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'done', 'failed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (event_id, consumer)
);

CREATE INDEX event_deliveries_claim_idx
    ON event_deliveries (consumer, available_at, created_at)
    WHERE status IN ('pending', 'failed');

-- Order contact is an immutable receipt destination. It intentionally avoids
-- resolving a mutable customer profile after payment and supports guest orders.
CREATE TABLE order_contact_details (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    email VARCHAR(320) NOT NULL,
    locale VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (email <> ''),
    CHECK (locale <> '')
);
