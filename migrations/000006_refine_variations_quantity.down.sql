-- 000006_refine_variations_quantity.down.sql
ALTER TABLE product_variation DROP COLUMN IF EXISTS quantity_value;
ALTER TABLE product_variation DROP COLUMN IF EXISTS unit_id;

-- Перетворюємо назад у VARCHAR тільки якщо колонка є JSONB
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns WHERE table_name='unit' AND column_name='name') = 'jsonb' THEN
        ALTER TABLE unit ALTER COLUMN name TYPE VARCHAR(50) USING name->>'uk';
        ALTER TABLE unit ALTER COLUMN short_name TYPE VARCHAR(10) USING short_name->>'uk';
    END IF;
END $$;
