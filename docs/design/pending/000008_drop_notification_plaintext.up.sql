-- Stages 4 and 5 of docs/design/notification-payload-encryption.md.
--
-- APPLY THIS ONLY AFTER `go run ./cmd/cli notifications-reencrypt` reports
-- nothing left in plaintext. The guard below stops it otherwise rather than
-- letting the DROP take the recipient and the rendered body of every job the
-- backfill has not reached — those columns are the only copy.
DO $$
DECLARE remaining BIGINT;
BEGIN
    SELECT COUNT(*) INTO remaining
    FROM notification_jobs
    WHERE dispatcher = 'scheduler' AND payload IS NOT NULL;
    IF remaining > 0 THEN
        RAISE EXCEPTION
            'notification_jobs still holds % scheduled jobs in plaintext; run: go run ./cmd/cli notifications-reencrypt', remaining;
    END IF;
END $$;

-- Stage 4: the shape is now the same for both dispatchers — everything to send
-- lives in the ciphertext — so the constraint that distinguished them has
-- nothing left to say.
ALTER TABLE notification_jobs
    DROP CONSTRAINT notification_jobs_dispatcher_payload_check;

ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_payload_present_check
    CHECK (payload_ciphertext <> '' AND payload_ciphertext <> '{}');

-- Stage 5. What stays readable is what operating the queue needs: type, locale,
-- status, dispatcher, timestamps, and the sanitized error code.
ALTER TABLE notification_jobs
    DROP COLUMN IF EXISTS payload,
    DROP COLUMN IF EXISTS recipient_email,
    DROP COLUMN IF EXISTS recipient;
