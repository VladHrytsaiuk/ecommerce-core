-- Removes the seeded row only while nothing points at it. stock_items and
-- inventory_reservations hold ON DELETE RESTRICT foreign keys, so an
-- unconditional delete would fail the rollback of a store that actually used
-- this warehouse — and rolling back a seed must not be the thing that destroys
-- a store's stock.
DELETE FROM warehouses AS w
WHERE w.id = '00000000-0000-4000-8000-000000000001'
  AND NOT EXISTS (SELECT 1 FROM stock_items AS s WHERE s.warehouse_id = w.id)
  AND NOT EXISTS (SELECT 1 FROM inventory_reservations AS r WHERE r.warehouse_id = w.id);
