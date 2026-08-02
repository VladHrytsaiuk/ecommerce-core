-- 000006_refine_variations_quantity.up.sql
DO $$ 
BEGIN
    -- Додаємо колонки тільки якщо вони не існують
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='product_variation' AND column_name='quantity_value') THEN
        ALTER TABLE product_variation ADD COLUMN quantity_value NUMERIC(10, 2);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='product_variation' AND column_name='unit_id') THEN
        ALTER TABLE product_variation ADD COLUMN unit_id INT REFERENCES unit(id);
    END IF;
END $$;

-- Перетворюємо існуючі колонки в JSONB, якщо вони ще VARCHAR
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns WHERE table_name='unit' AND column_name='name') = 'character varying' THEN
        ALTER TABLE unit ALTER COLUMN name TYPE JSONB USING json_build_object('uk', name);
        ALTER TABLE unit ALTER COLUMN short_name TYPE JSONB USING json_build_object('uk', short_name);
    END IF;
END $$;

-- Оновлюємо дані одиниць виміру
TRUNCATE unit RESTART IDENTITY CASCADE;
INSERT INTO unit (name, short_name) VALUES 
('{"uk": "мілілітри", "en": "milliliters"}', '{"uk": "мл", "en": "ml"}'),
('{"uk": "літри", "en": "liters"}', '{"uk": "л", "en": "l"}'),
('{"uk": "грами", "en": "grams"}', '{"uk": "г", "en": "g"}'),
('{"uk": "кілограми", "en": "kilograms"}', '{"uk": "кг", "en": "kg"}'),
('{"uk": "штуки", "en": "pieces"}', '{"uk": "шт", "en": "pcs"}');
