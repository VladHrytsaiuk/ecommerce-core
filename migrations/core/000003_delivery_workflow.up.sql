-- Provider-neutral delivery snapshot and durable work queue. These records are
-- created inside the order workflow transaction; a worker performs carrier I/O
-- only after the transaction commits.
CREATE TABLE order_delivery_details (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    recipient_phone VARCHAR(32) NOT NULL,
    country_code CHAR(2),
    postal_code VARCHAR(32),
    city TEXT,
    line1 TEXT,
    line2 TEXT,
    locality_id VARCHAR(255),
    service_point_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE delivery_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    idempotency_key UUID NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'retrying', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX delivery_jobs_ready_idx ON delivery_jobs(status, available_at)
    WHERE status IN ('pending', 'retrying');
