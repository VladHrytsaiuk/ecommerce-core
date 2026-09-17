-- 000037_update_bundle_unit.up.sql

UPDATE attribute 
SET unit_id = (SELECT id FROM unit WHERE name->>'uk' = 'товари' LIMIT 1)
WHERE code = 'bundle_items_count';

UPDATE attribute_value
SET unit_id = (SELECT id FROM unit WHERE name->>'uk' = 'товари' LIMIT 1)
WHERE attribute_id = (SELECT id FROM attribute WHERE code = 'bundle_items_count' LIMIT 1);
