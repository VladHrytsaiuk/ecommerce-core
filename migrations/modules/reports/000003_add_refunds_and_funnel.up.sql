CREATE TABLE report_daily_funnel (
    bucket_date DATE NOT NULL,
    channel VARCHAR(64) NOT NULL,
    carts_created BIGINT NOT NULL DEFAULT 0 CHECK (carts_created >= 0),
    checkouts_started BIGINT NOT NULL DEFAULT 0 CHECK (checkouts_started >= 0),
    orders_paid BIGINT NOT NULL DEFAULT 0 CHECK (orders_paid >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT report_daily_funnel_bucket_unique UNIQUE (bucket_date, channel),
    CHECK (btrim(channel) <> '')
);
CREATE INDEX report_daily_funnel_date_idx ON report_daily_funnel (bucket_date, channel);

-- A refund delivery can arrive before a historic paid projection after a
-- rebuild/replay. Product gross and units remain non-negative; net is signed.
ALTER TABLE report_daily_product_sales
    DROP CONSTRAINT IF EXISTS report_daily_product_sales_net_equals_gross_check;
