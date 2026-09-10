-- Claim matches two disjoint sets of rows: deliveries that are due, and
-- deliveries whose processing lease expired because a worker died. Only the
-- first was indexed, so the OR forced a sequential scan of the whole table on
-- every poll — once per interval, per consumer, per replica, and growing with
-- the table even when the queue is empty.
CREATE INDEX CONCURRENTLY IF NOT EXISTS event_deliveries_lease_reclaim_idx
    ON event_deliveries (consumer, locked_at)
    WHERE status = 'processing';
