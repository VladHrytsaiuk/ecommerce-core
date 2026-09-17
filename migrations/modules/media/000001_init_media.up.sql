-- Media owns object metadata and processing history. Object bytes live only in
-- a configured object-store provider; this module never stores image blobs in PostgreSQL.
CREATE TABLE media_assets (
    id UUID PRIMARY KEY,
    upload_id UUID NOT NULL UNIQUE,
    provider VARCHAR(32) NOT NULL,
    bucket VARCHAR(255) NOT NULL,
    object_key TEXT NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status IN ('quarantine', 'processing', 'ready', 'failed')),
    checksum_sha256 CHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    mime_type VARCHAR(127) NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    created_by UUID,
    failure_code VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider, bucket, object_key),
    CHECK (checksum_sha256 ~ '^[a-f0-9]{64}$')
);
CREATE INDEX media_assets_status_created_idx ON media_assets (status, created_at);

CREATE TABLE media_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    variant_key VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    bucket VARCHAR(255) NOT NULL,
    object_key TEXT NOT NULL,
    width INTEGER CHECK (width > 0),
    height INTEGER CHECK (height > 0),
    size_bytes BIGINT CHECK (size_bytes > 0),
    mime_type VARCHAR(127) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (asset_id, variant_key),
    UNIQUE (provider, bucket, object_key)
);

CREATE TABLE media_processing_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    event_id UUID NOT NULL UNIQUE,
    attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt > 0),
    status VARCHAR(32) NOT NULL CHECK (status IN ('processing', 'success', 'failed')),
    failure_code VARCHAR(128),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    UNIQUE (asset_id, attempt)
);
CREATE INDEX media_processing_attempts_asset_idx ON media_processing_attempts (asset_id, started_at DESC);
