ALTER TABLE payment_checkout_attempts
    ADD COLUMN locked_at TIMESTAMPTZ;

ALTER TABLE payment_checkout_attempts
    DROP CONSTRAINT payment_checkout_attempts_status_check,
    ADD CONSTRAINT payment_checkout_attempts_status_check
        CHECK (status IN ('creating', 'processing', 'created', 'failed'));

CREATE INDEX payment_checkout_attempts_processing_idx
    ON payment_checkout_attempts(status, locked_at)
    WHERE status = 'processing';
