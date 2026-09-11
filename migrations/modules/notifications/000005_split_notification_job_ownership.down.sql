DROP INDEX notification_jobs_scheduler_reclaim_idx;
DROP INDEX notification_jobs_scheduler_due_idx;

CREATE INDEX notification_jobs_due_idx ON notification_jobs (status, next_retry_at, created_at)
    WHERE status IN ('pending', 'failed');

CREATE INDEX notification_jobs_claim_idx
    ON notification_jobs (status, locked_at, created_at)
    WHERE status IN ('pending', 'sending', 'failed');

ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_dispatcher_payload_check;
ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_dispatcher_check;
ALTER TABLE notification_jobs DROP COLUMN dispatcher;
