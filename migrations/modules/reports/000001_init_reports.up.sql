-- Reports owns only CQRS read models. These aggregates are intentionally
-- separate from Core OLTP tables and are updated idempotently from Outbox.
CREATE TABLE report_processed_events (
    event_id UUID PRIMARY KEY,
    topic VARCHAR(128) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (btrim(topic) <> '')
);
CREATE INDEX report_processed_events_processed_at_idx
    ON report_processed_events (processed_at);

CREATE TABLE report_daily_sales (
    bucket_date DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    channel VARCHAR(64) NOT NULL,
    paid_orders_count BIGINT NOT NULL DEFAULT 0 CHECK (paid_orders_count >= 0),
    cancelled_orders_count BIGINT NOT NULL DEFAULT 0 CHECK (cancelled_orders_count >= 0),
    refunded_orders_count BIGINT NOT NULL DEFAULT 0 CHECK (refunded_orders_count >= 0),
    gross_revenue_minor BIGINT NOT NULL DEFAULT 0 CHECK (gross_revenue_minor >= 0),
    refund_revenue_minor BIGINT NOT NULL DEFAULT 0 CHECK (refund_revenue_minor >= 0),
    net_revenue_minor BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT report_daily_sales_bucket_unique
        UNIQUE (bucket_date, timezone, currency, channel),
    CHECK (btrim(timezone) <> ''),
    CHECK (btrim(currency) <> ''),
    CHECK (btrim(channel) <> ''),
    CHECK (net_revenue_minor = gross_revenue_minor - refund_revenue_minor)
);
CREATE INDEX report_daily_sales_date_timezone_idx
    ON report_daily_sales (bucket_date DESC, timezone, currency, channel);
