-- A job lease prevents a reclaimed outbox delivery from concurrently sending
-- the same email. lock_token fences a stale sender from completing a newer
-- claim, while dead retains terminal failures for operational triage.
ALTER TABLE notification_jobs
    ADD COLUMN locked_at TIMESTAMPTZ,
    ADD COLUMN lock_token UUID;

ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'dead'));

CREATE INDEX notification_jobs_claim_idx
    ON notification_jobs (status, locked_at, created_at)
    WHERE status IN ('pending', 'sending', 'failed');
