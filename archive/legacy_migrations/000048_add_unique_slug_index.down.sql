DROP INDEX IF EXISTS idx_category_translation_slug_unique;
DROP INDEX IF EXISTS idx_product_translation_slug_unique;

CREATE INDEX IF NOT EXISTS idx_category_translation_slug ON category_translation (slug);
CREATE INDEX IF NOT EXISTS idx_product_translation_slug ON product_translation (slug);
