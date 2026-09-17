-- Cleanup leases make stale Direct Upload deletion crash-safe: only one worker
-- may call the provider for an asset, and stale leases are reclaimed later.
ALTER TABLE video_assets DROP CONSTRAINT IF EXISTS video_assets_status_check;
ALTER TABLE video_assets
    ADD CONSTRAINT video_assets_status_check
    CHECK (status IN ('draft', 'uploading', 'processing', 'ready', 'failed', 'deleting', 'deleted'));

CREATE INDEX video_assets_cleanup_candidates_idx
    ON video_assets (updated_at ASC)
    WHERE status IN ('draft', 'uploading', 'processing', 'deleting');
