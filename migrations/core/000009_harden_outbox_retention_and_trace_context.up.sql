-- Trace context is transport metadata, separate from immutable business JSON.
ALTER TABLE domain_events
    ADD COLUMN traceparent VARCHAR(512),
    ADD COLUMN tracestate VARCHAR(512),
    ADD COLUMN request_id VARCHAR(128);

-- Terminal delivery history is moved to the archive in bounded batches. The
-- hot table retains only work that needs operational attention.
CREATE TABLE event_delivery_archive (
    event_id UUID NOT NULL,
    consumer VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    attempts INTEGER NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    last_error TEXT,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (event_id, consumer)
);
CREATE INDEX event_delivery_archive_completed_idx
    ON event_delivery_archive (completed_at DESC);

CREATE INDEX event_deliveries_done_retention_idx
    ON event_deliveries (completed_at)
    WHERE status = 'done' AND completed_at IS NOT NULL;

-- High-churn status transitions otherwise leave dead tuples faster than the
-- default scale-factor can vacuum on a busy store.
ALTER TABLE event_deliveries SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_analyze_scale_factor = 0.005,
    autovacuum_vacuum_threshold = 1000,
    autovacuum_analyze_threshold = 500
);
