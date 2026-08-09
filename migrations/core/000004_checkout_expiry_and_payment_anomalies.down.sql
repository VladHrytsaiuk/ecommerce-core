DROP TABLE IF EXISTS payment_anomalies;
DROP INDEX IF EXISTS orders_pending_payment_expiry_idx;
ALTER TABLE payment_checkout_attempts ALTER COLUMN expires_at DROP DEFAULT;
ALTER TABLE orders ALTER COLUMN expires_at DROP DEFAULT;
ALTER TABLE payment_checkout_attempts DROP COLUMN IF EXISTS expires_at;
ALTER TABLE orders DROP COLUMN IF EXISTS expires_at;
