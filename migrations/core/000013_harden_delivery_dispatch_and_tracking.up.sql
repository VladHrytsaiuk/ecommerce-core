-- Phase 17: delivery jobs are retained for manual reconciliation after a
-- bounded retry budget, and delivery tracking has neutral shipped/received
-- states. One order/provider pair has one durable shipment in the current
-- single-parcel delivery model.
ALTER TABLE delivery_jobs
    DROP CONSTRAINT IF EXISTS delivery_jobs_status_check;
ALTER TABLE delivery_jobs
    ADD CONSTRAINT delivery_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'retrying', 'failed', 'dead'));

ALTER TABLE deliveries
    DROP CONSTRAINT IF EXISTS deliveries_status_check;
ALTER TABLE deliveries
    ADD CONSTRAINT deliveries_status_check
    CHECK (status IN ('pending', 'created', 'in_transit', 'shipped', 'delivered', 'received', 'failed', 'cancelled'));

ALTER TABLE deliveries
    ADD COLUMN IF NOT EXISTS provider_reference VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS deliveries_order_provider_unique
    ON deliveries (order_id, provider);
