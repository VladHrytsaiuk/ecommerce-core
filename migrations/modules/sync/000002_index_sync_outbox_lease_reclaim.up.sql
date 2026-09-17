-- Claim matches two disjoint sets: events that are due, and events whose lease
-- expired because a dispatcher died. Only the first was indexed, so the OR
-- between them forced a sequential scan of sync_outbox on every poll. This is
-- the same gap that event_deliveries carried in core migration 000015.
CREATE INDEX CONCURRENTLY IF NOT EXISTS sync_outbox_lease_reclaim_idx
    ON sync_outbox (locked_at)
    WHERE status = 'processing';
