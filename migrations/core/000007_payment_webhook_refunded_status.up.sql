ALTER TABLE payment_webhook_events DROP CONSTRAINT IF EXISTS payment_webhook_events_event_status_check;
ALTER TABLE payment_webhook_events ADD CONSTRAINT payment_webhook_events_event_status_check
    CHECK (event_status IN ('pending', 'paid', 'failed', 'cancelled', 'expired', 'refunded'));
