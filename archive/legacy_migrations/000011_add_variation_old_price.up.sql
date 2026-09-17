-- 000011_add_variation_old_price.up.sql
ALTER TABLE product_variation ADD COLUMN old_price INT;
COMMENT ON COLUMN product_variation.old_price IS 'Original price before discount (in minimal units, e.g. cents/kopecks)';
