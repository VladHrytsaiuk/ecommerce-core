-- Sync owns integration delivery state. Aggregate IDs are logical references
-- to Core or module records: no cross-module foreign keys are declared here.
-- `payload` is a documented, non-localized JSON transport envelope stored as
-- text, never catalog content or a source of truth for the local read model.
CREATE TABLE sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic VARCHAR(128) NOT NULL,
    aggregate_id UUID NOT NULL,
    idempotency_key UUID NOT NULL,
    payload TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'delivered', 'failed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ,
    last_error TEXT,
    delivered_at TIMESTAMPTZ,
    dead_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (topic, idempotency_key)
);
CREATE INDEX sync_outbox_pending_idx
    ON sync_outbox (available_at, created_at)
    WHERE status IN ('pending', 'failed');

-- A source-specific version and payload hash make duplicate and stale inbound
-- ERP updates deterministic. The application layer compares them before
-- changing Catalog or Inventory read models.
CREATE TABLE sync_external_entity_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source VARCHAR(64) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    external_id VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    source_updated_at TIMESTAMPTZ,
    payload_hash CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'applied'
        CHECK (status IN ('received', 'applied', 'rejected', 'failed')),
    locked_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source, entity_type, external_id)
);
CREATE INDEX sync_external_entity_state_source_updated_idx
    ON sync_external_entity_state (source, entity_type, source_updated_at DESC);

-- Cursors keep pull-based integrations independent from the entities they
-- import and allow a worker to resume after restart.
CREATE TABLE sync_cursors (
    source VARCHAR(64) NOT NULL,
    stream VARCHAR(128) NOT NULL,
    cursor VARCHAR(1024) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source, stream)
);
