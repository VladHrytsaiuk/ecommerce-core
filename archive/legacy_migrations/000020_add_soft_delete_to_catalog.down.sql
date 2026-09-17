DROP INDEX IF EXISTS idx_unit_deleted_at;
ALTER TABLE unit DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_product_deleted_at;
ALTER TABLE product DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_attribute_deleted_at;
ALTER TABLE attribute DROP COLUMN IF EXISTS deleted_at;
