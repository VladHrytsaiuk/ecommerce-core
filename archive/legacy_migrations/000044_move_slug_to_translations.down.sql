-- 1. Add slug back to category
ALTER TABLE category ADD COLUMN slug VARCHAR(255);

-- Copy back from category_translation (just pick the 'uk' one)
UPDATE category c
SET slug = ct.slug
FROM category_translation ct
WHERE ct.category_id = c.id AND ct.language_code = 'uk';

-- Make NOT NULL
ALTER TABLE category ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX idx_category_slug ON category (slug) WHERE deleted_at IS NULL;

-- Drop from category_translation
DROP INDEX IF EXISTS idx_category_translation_slug;
ALTER TABLE category_translation DROP COLUMN IF EXISTS slug;

-- 2. Add slug back to product
ALTER TABLE product ADD COLUMN slug VARCHAR(255);

-- Copy back from product_translation (just pick the 'uk' one)
UPDATE product p
SET slug = pt.slug
FROM product_translation pt
WHERE pt.product_id = p.id AND pt.language_code = 'uk';

-- Make NOT NULL
ALTER TABLE product ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX idx_product_slug ON product (slug) WHERE deleted_at IS NULL;

-- Drop from product_translation
DROP INDEX IF EXISTS idx_product_translation_slug;
ALTER TABLE product_translation DROP COLUMN IF EXISTS slug;
