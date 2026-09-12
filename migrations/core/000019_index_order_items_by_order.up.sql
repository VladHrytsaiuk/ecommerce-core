-- A REFERENCES clause gives referential integrity, not an index: PostgreSQL,
-- unlike MySQL, does not index a foreign key column automatically. order_items
-- has been read by order_id on three paths since it was created — creating a
-- shipment, filing a return, and snapshotting an order for analytics — and
-- every one of those has been a sequential scan of the whole table.
--
-- The table grows one row per order line and is a financial record, so it is
-- never pruned. The scan therefore gets slower for the life of the store.
CREATE INDEX order_items_order_idx ON order_items (order_id);
