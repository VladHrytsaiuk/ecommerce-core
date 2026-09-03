-- Video owns provider-facing video asset state. product_id intentionally has
-- no foreign key: Catalog remains an isolated module.
CREATE TABLE video_assets (
    id UUID PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    external_id VARCHAR(255) UNIQUE,
    status VARCHAR(32) NOT NULL CHECK (status IN ('draft', 'uploading', 'processing', 'ready', 'failed', 'deleted')),
    duration_seconds INTEGER CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    poster_url VARCHAR(2048),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX video_assets_status_updated_idx ON video_assets (status, updated_at DESC);

CREATE TABLE product_videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL,
    video_asset_id UUID NOT NULL REFERENCES video_assets(id) ON DELETE RESTRICT,
    role VARCHAR(32) NOT NULL CHECK (role IN ('preview', 'how_to_use')),
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    is_visible BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (product_id, role, position)
);
CREATE INDEX product_videos_product_visible_position_idx ON product_videos (product_id, is_visible, position);
