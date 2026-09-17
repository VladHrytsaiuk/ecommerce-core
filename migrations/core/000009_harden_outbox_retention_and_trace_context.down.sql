ALTER TABLE event_deliveries RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_analyze_scale_factor,
    autovacuum_vacuum_threshold,
    autovacuum_analyze_threshold
);
DROP INDEX IF EXISTS event_deliveries_done_retention_idx;
DROP TABLE IF EXISTS event_delivery_archive;
ALTER TABLE domain_events
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS tracestate,
    DROP COLUMN IF EXISTS traceparent;
