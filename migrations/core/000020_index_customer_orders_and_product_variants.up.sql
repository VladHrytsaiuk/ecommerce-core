-- Two more foreign keys read by equality on hot paths with no index behind
-- them. Both were measured on twenty thousand rows before this migration:
-- Seq Scan discarding every row, twice.
--
-- orders.customer_id backs the customer's own order history — a COUNT and a
-- paginated, ordered SELECT. orders is the largest financial table here and is
-- never pruned, so this scan gets slower for the life of the store. The index
-- carries the sort so the pagination does not have to re-sort the match set.
CREATE INDEX orders_customer_recent_idx
    ON orders (customer_id, created_at DESC, id DESC)
    WHERE customer_id IS NOT NULL;

-- product_variants.product_id is read whenever a product is loaded with its
-- variants, which is the most visited page a store has.
CREATE INDEX product_variants_product_idx ON product_variants (product_id);
