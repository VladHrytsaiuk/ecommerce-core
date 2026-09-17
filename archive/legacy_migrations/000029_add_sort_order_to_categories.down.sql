DROP INDEX IF EXISTS idx_category_parent_sort_order;
DROP INDEX IF EXISTS idx_category_root_sort_order;
ALTER TABLE category DROP COLUMN IF EXISTS sort_order;
