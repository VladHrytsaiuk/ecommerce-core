-- Confirming, cancelling or expiring an order looks its reservations up by
-- order_id. There was no index for that predicate: the only two on this table
-- are partial and keyed on expires_at and updated_at. EXPLAIN (ANALYZE) on a
-- table of twenty thousand rows showed a sequential scan discarding all of
-- them to find one order's reservations, on every payment confirmation.
--
-- Partial on order_id IS NOT NULL because a reservation carries no order until
-- checkout attaches one, and this lookup never wants those rows. That also
-- keeps the index off the hot path that creates reservations.
CREATE INDEX inventory_reservations_order_idx
    ON inventory_reservations (order_id)
    WHERE order_id IS NOT NULL;
