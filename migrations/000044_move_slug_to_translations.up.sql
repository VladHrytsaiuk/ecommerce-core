-- 1. Add slug to category_translation
ALTER TABLE category_translation ADD COLUMN slug VARCHAR(255);

-- Copy existing slugs from category to category_translation (only uk and en)
-- Append language code and random string to avoid unique constraint violations during migration
UPDATE category_translation ct
SET slug = c.slug || CASE WHEN ct.language_code = 'en' THEN '-en' ELSE '' END || '-' || substr(md5(random()::text), 1, 4)
FROM category c
WHERE ct.category_id = c.id;

-- Make slug NOT NULL
ALTER TABLE category_translation ALTER COLUMN slug SET NOT NULL;

-- Create Index
CREATE INDEX idx_category_translation_slug ON category_translation (slug);

-- Drop slug from category
DROP INDEX IF EXISTS idx_category_slug;
ALTER TABLE category DROP COLUMN IF EXISTS slug;

-- 2. Add slug to product_translation
ALTER TABLE product_translation ADD COLUMN slug VARCHAR(255);

-- Copy existing slugs from product to product_translation (only uk and en)
-- Append language code and random string to avoid unique constraint violations during migration
UPDATE product_translation pt
SET slug = p.slug || CASE WHEN pt.language_code = 'en' THEN '-en' ELSE '' END || '-' || substr(md5(random()::text), 1, 4)
FROM product p
WHERE pt.product_id = p.id;

-- Make slug NOT NULL
ALTER TABLE product_translation ALTER COLUMN slug SET NOT NULL;

-- Create Index
CREATE INDEX idx_product_translation_slug ON product_translation (slug);

-- Drop slug from product
DROP INDEX IF EXISTS idx_product_slug;
ALTER TABLE product DROP COLUMN IF EXISTS slug;
