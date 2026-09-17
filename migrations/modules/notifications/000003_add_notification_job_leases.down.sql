DROP INDEX IF EXISTS notification_jobs_claim_idx;
ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
UPDATE notification_jobs SET status = 'failed' WHERE status IN ('sending', 'dead');
ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending', 'sent', 'failed'));
ALTER TABLE notification_jobs
    DROP COLUMN IF EXISTS lock_token,
    DROP COLUMN IF EXISTS locked_at;
