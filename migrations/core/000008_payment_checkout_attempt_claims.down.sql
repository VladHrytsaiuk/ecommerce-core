DROP INDEX IF EXISTS payment_checkout_attempts_processing_idx;

ALTER TABLE payment_checkout_attempts
    DROP CONSTRAINT payment_checkout_attempts_status_check,
    ADD CONSTRAINT payment_checkout_attempts_status_check
        CHECK (status IN ('creating', 'created', 'failed')),
    DROP COLUMN locked_at;
