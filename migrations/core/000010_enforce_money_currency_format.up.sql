-- Money is persisted as minor units plus ISO-4217 currency. These constraints
-- are NOT VALID so an existing installation can deploy safely; PostgreSQL still
-- enforces them for every new or modified row. Historical invalid rows are
-- surfaced as controlled mapping errors until a dedicated repair is run.
ALTER TABLE product_variants
    ADD CONSTRAINT product_variants_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE orders
    ADD CONSTRAINT orders_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE order_items
    ADD CONSTRAINT order_items_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE payments
    ADD CONSTRAINT payments_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE payment_checkout_attempts
    ADD CONSTRAINT payment_checkout_attempts_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE payment_webhook_events
    ADD CONSTRAINT payment_webhook_events_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;

ALTER TABLE payment_anomalies
    ADD CONSTRAINT payment_anomalies_currency_iso_check
    CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;
