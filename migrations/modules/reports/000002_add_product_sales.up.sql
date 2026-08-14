CREATE TABLE report_daily_product_sales (
    bucket_date DATE NOT NULL,
    currency VARCHAR(3) NOT NULL,
    product_id UUID NOT NULL,
    variant_id UUID,
    units_sold BIGINT NOT NULL DEFAULT 0 CHECK (units_sold >= 0),
    gross_revenue_minor BIGINT NOT NULL DEFAULT 0 CHECK (gross_revenue_minor >= 0),
    net_revenue_minor BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT report_daily_product_sales_bucket_unique
        UNIQUE NULLS NOT DISTINCT (bucket_date, currency, product_id, variant_id),
    CONSTRAINT report_daily_product_sales_net_equals_gross_check
        CHECK (net_revenue_minor = gross_revenue_minor)
);
CREATE INDEX report_daily_product_sales_top_revenue_idx
    ON report_daily_product_sales (bucket_date, currency, net_revenue_minor DESC, product_id);
CREATE INDEX report_daily_product_sales_top_units_idx
    ON report_daily_product_sales (bucket_date, currency, units_sold DESC, product_id);
