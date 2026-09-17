-- 000034_add_bundle_attributes.down.sql

DELETE FROM attribute WHERE code IN ('bundle_items_count', 'bundle_items');
