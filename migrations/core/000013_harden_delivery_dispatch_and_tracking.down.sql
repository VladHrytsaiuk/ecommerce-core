DROP INDEX IF EXISTS deliveries_order_provider_unique;
ALTER TABLE deliveries DROP COLUMN IF EXISTS provider_reference;
ALTER TABLE deliveries DROP CONSTRAINT IF EXISTS deliveries_status_check;
ALTER TABLE deliveries
    ADD CONSTRAINT deliveries_status_check
    CHECK (status IN ('pending', 'created', 'in_transit', 'delivered', 'failed', 'cancelled'));
ALTER TABLE delivery_jobs DROP CONSTRAINT IF EXISTS delivery_jobs_status_check;
ALTER TABLE delivery_jobs
    ADD CONSTRAINT delivery_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'retrying', 'failed'));
