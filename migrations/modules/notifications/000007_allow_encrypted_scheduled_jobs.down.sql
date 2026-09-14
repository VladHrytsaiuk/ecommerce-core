-- Narrowing back rejects any scheduler job already re-encrypted by stage 3, so
-- this only succeeds before that runs — or after those rows are gone.
ALTER TABLE notification_jobs
    DROP CONSTRAINT notification_jobs_dispatcher_payload_check;

ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_dispatcher_payload_check
    CHECK ((dispatcher = 'scheduler' AND payload IS NOT NULL)
        OR (dispatcher = 'outbox' AND payload IS NULL));
