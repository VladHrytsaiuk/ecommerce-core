ALTER TABLE orders ADD COLUMN expires_at TIMESTAMPTZ;
ALTER TABLE payment_checkout_attempts ADD COLUMN expires_at TIMESTAMPTZ;
UPDATE orders SET expires_at = created_at + INTERVAL '30 minutes' WHERE status = 'pending_payment' AND expires_at IS NULL;
UPDATE payment_checkout_attempts SET expires_at = created_at + INTERVAL '30 minutes' WHERE expires_at IS NULL;
ALTER TABLE orders ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE payment_checkout_attempts ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE orders ALTER COLUMN expires_at SET DEFAULT CURRENT_TIMESTAMP + INTERVAL '30 minutes';
ALTER TABLE payment_checkout_attempts ALTER COLUMN expires_at SET DEFAULT CURRENT_TIMESTAMP + INTERVAL '30 minutes';
ALTER TABLE payment_webhook_events DROP CONSTRAINT IF EXISTS payment_webhook_events_event_status_check;
ALTER TABLE payment_webhook_events ADD CONSTRAINT payment_webhook_events_event_status_check CHECK (event_status IN ('pending','paid','failed','cancelled','expired'));
CREATE INDEX orders_pending_payment_expiry_idx ON orders(expires_at) WHERE status = 'pending_payment';
CREATE TABLE payment_anomalies (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
 provider VARCHAR(64) NOT NULL,
 provider_reference VARCHAR(255) NOT NULL,
 amount BIGINT NOT NULL CHECK(amount >= 0),
 currency CHAR(3) NOT NULL,
 reason VARCHAR(64) NOT NULL CHECK(reason IN ('paid_after_cancelled')),
 status VARCHAR(32) NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 resolved_at TIMESTAMPTZ,
 UNIQUE(order_id, provider, provider_reference, reason)
);
CREATE INDEX payment_anomalies_open_idx ON payment_anomalies(created_at) WHERE status = 'open';
