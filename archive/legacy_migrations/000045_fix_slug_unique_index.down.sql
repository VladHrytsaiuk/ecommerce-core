DROP INDEX IF EXISTS idx_category_translation_slug;
DROP INDEX IF EXISTS idx_product_translation_slug;

CREATE UNIQUE INDEX idx_category_translation_slug ON category_translation (slug);
CREATE UNIQUE INDEX idx_product_translation_slug ON product_translation (slug);
