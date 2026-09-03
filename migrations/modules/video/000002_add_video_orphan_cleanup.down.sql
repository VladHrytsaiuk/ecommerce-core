DROP INDEX IF EXISTS video_assets_cleanup_candidates_idx;

ALTER TABLE video_assets DROP CONSTRAINT IF EXISTS video_assets_status_check;
ALTER TABLE video_assets
    ADD CONSTRAINT video_assets_status_check
    CHECK (status IN ('draft', 'uploading', 'processing', 'ready', 'failed', 'deleted'));
