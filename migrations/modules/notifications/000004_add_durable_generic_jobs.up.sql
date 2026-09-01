ALTER TABLE notification_jobs
    ADD COLUMN type VARCHAR(64) NOT NULL DEFAULT 'order_paid',
    ADD COLUMN recipient_email VARCHAR(320),
    ADD COLUMN payload TEXT,
    ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    ADD COLUMN next_retry_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE notification_jobs ALTER COLUMN order_id DROP NOT NULL;
UPDATE notification_jobs SET recipient_email = recipient WHERE recipient_email IS NULL;
CREATE INDEX notification_jobs_due_idx ON notification_jobs (status, next_retry_at, created_at)
    WHERE status IN ('pending', 'failed');
