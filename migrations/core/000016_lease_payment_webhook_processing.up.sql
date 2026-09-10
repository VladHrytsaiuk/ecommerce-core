-- Claiming a webhook fell back to reading processing_status without a lock, so
-- two replicas receiving the same provider retry could both see 'processing'
-- and handle the callback at the same time. The order workflow is idempotent,
-- so money was never at risk, but the loser logged a payment failure that
-- never happened. A lease makes the claim exclusive while still allowing a
-- crashed replica's work to be picked up.
ALTER TABLE payment_webhook_events
    ADD COLUMN locked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- Rows already processed hold no lease; keeping one would only confuse
-- operators reading the table.
UPDATE payment_webhook_events SET locked_at = received_at WHERE processing_status = 'processing';
