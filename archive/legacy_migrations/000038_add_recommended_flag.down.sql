DROP INDEX IF EXISTS idx_product_is_recommended;
ALTER TABLE product DROP COLUMN IF EXISTS is_recommended;
