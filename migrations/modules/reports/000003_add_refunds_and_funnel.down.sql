DROP TABLE IF EXISTS report_daily_funnel;

ALTER TABLE report_daily_product_sales
    ADD CONSTRAINT report_daily_product_sales_net_equals_gross_check
    CHECK (net_revenue_minor = gross_revenue_minor);
