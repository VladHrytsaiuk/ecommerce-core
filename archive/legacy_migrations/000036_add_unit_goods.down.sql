-- 000036_add_unit_goods.down.sql

DELETE FROM unit WHERE name->>'uk' = 'товари';
