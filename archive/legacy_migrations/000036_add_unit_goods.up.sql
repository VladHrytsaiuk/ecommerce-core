-- 000036_add_unit_goods.up.sql

INSERT INTO unit (name, short_name) 
SELECT '{"uk": "товари", "en": "products"}'::jsonb, '{"uk": "шт", "en": "pcs"}'::jsonb
WHERE NOT EXISTS (
    SELECT 1 FROM unit WHERE name->>'uk' = 'товари'
);
