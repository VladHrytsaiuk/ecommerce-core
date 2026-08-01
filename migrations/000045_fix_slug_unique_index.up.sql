-- Drop the unique constraint if it exists (it was created in the earlier version of 000044)
DROP INDEX IF EXISTS idx_category_translation_slug;
DROP INDEX IF EXISTS idx_product_translation_slug;

-- Create normal index
CREATE INDEX IF NOT EXISTS idx_category_translation_slug ON category_translation (slug);
CREATE INDEX IF NOT EXISTS idx_product_translation_slug ON product_translation (slug);
