-- Stage 1 of encrypting every notification job. See
-- docs/design/notification-payload-encryption.md for the whole sequence.
--
-- The existing constraint requires a scheduler job to carry a plaintext
-- payload, which makes writing ciphertext impossible: the dual-write in stage 2
-- cannot run until this is relaxed. It stays a real constraint — an outbox job
-- still may not carry plaintext, and every job must carry something to render
-- from — it simply stops forbidding the shape the next stage introduces.
--
-- This migration is safe to apply on its own. It widens what is accepted and
-- rejects nothing that was accepted before.
ALTER TABLE notification_jobs
    DROP CONSTRAINT notification_jobs_dispatcher_payload_check;

ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_dispatcher_payload_check
    CHECK (
        -- An outbox job has always carried ciphertext and no plaintext.
        (dispatcher = 'outbox' AND payload IS NULL)
        -- A scheduler job carries one or the other: plaintext until stage 3
        -- re-encrypts it, ciphertext afterwards. Never neither.
        OR (dispatcher = 'scheduler' AND (payload IS NOT NULL OR payload_ciphertext <> '{}'))
    );
