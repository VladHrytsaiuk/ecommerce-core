-- 000037_update_bundle_unit.down.sql

UPDATE attribute 
SET unit_id = 5
WHERE code = 'bundle_items_count';

UPDATE attribute_value
SET unit_id = 5
WHERE attribute_id = (SELECT id FROM attribute WHERE code = 'bundle_items_count' LIMIT 1);
