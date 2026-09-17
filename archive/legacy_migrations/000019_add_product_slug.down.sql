DROP INDEX IF EXISTS idx_attribute_value_code;
ALTER TABLE attribute_value DROP COLUMN IF EXISTS value_code;

DROP INDEX IF EXISTS idx_product_slug;
ALTER TABLE product DROP COLUMN IF EXISTS slug;
