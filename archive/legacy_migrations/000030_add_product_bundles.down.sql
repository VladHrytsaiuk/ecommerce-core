-- 000030_add_product_bundles.down.sql

-- Drop indexes
DROP INDEX IF EXISTS idx_product_variation_is_active_not_deleted;
DROP INDEX IF EXISTS idx_product_is_active_not_deleted;
DROP INDEX IF EXISTS idx_product_is_bundle;
DROP INDEX IF EXISTS idx_product_bundle_item_variation_id;

-- Drop junction table
DROP TABLE IF EXISTS product_bundle_item;

-- Remove columns from product
ALTER TABLE product DROP COLUMN IF EXISTS price_strategy;
ALTER TABLE product DROP COLUMN IF EXISTS is_bundle;
