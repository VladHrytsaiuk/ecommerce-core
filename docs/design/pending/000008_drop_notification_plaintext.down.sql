-- This rollback cannot restore what the up removed.
--
-- The recipient and the rendered payload of every scheduled job live only
-- inside payload_ciphertext now, and only the application can open it. These
-- columns come back empty; the rows keep working because the dispatcher reads
-- the ciphertext, but nothing here refills them.
ALTER TABLE notification_jobs
    DROP CONSTRAINT IF EXISTS notification_jobs_payload_present_check;

ALTER TABLE notification_jobs
    ADD COLUMN IF NOT EXISTS payload TEXT,
    ADD COLUMN IF NOT EXISTS recipient_email VARCHAR(320),
    ADD COLUMN IF NOT EXISTS recipient VARCHAR(320);

ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_dispatcher_payload_check
    CHECK (
        (dispatcher = 'outbox' AND payload IS NULL)
        OR (dispatcher = 'scheduler' AND (payload IS NOT NULL OR payload_ciphertext <> '{}'))
    );
