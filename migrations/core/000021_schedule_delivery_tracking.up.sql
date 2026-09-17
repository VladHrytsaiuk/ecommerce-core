-- The tracker read "active" deliveries with LIMIT 100 and no ordering, no
-- cursor and no lease. Three things followed from that.
--
-- Its filter named 'shipped', which is not one of this column's statuses, and
-- 'delivered', which is terminal — so every delivered shipment stayed in the
-- "active" set forever. Once a hundred of them accumulated, an arbitrary and
-- effectively fixed subset filled the window and shipments actually in transit
-- stopped being polled at all. The tracker went quiet as the store succeeded.
--
-- Without ordering, PostgreSQL was free to return the same rows every tick, so
-- even below the limit some deliveries could be polled forever and others never.
--
-- Without a lease, every replica polled the same rows and called the carrier's
-- API once per replica for each of them.
--
-- next_check_at is when a delivery is next due. The claim orders by it, takes a
-- bounded batch with FOR UPDATE SKIP LOCKED, and pushes the column forward, so
-- rows rotate, replicas take disjoint work, and a carrier sees one call per
-- delivery per cycle.
ALTER TABLE deliveries ADD COLUMN next_check_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- Partial, over exactly the rows the claim scans: a store's delivery history is
-- mostly terminal rows the tracker must never look at again.
CREATE INDEX deliveries_due_idx
    ON deliveries (next_check_at, id)
    WHERE tracking_number IS NOT NULL AND status IN ('pending', 'created', 'in_transit');
