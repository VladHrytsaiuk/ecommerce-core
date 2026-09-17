-- Add unique composite index on (slug, language_code) to prevent race condition duplicates

-- For category_translation: unique per language
DROP INDEX IF EXISTS idx_category_translation_slug;
CREATE UNIQUE INDEX idx_category_translation_slug_unique
    ON category_translation (slug, language_code);

-- For product_translation: unique per language
-- We use a simple unique index; soft-deleted products will have their slugs freed
-- by the application layer before reinserting.
DROP INDEX IF EXISTS idx_product_translation_slug;
CREATE UNIQUE INDEX idx_product_translation_slug_unique
    ON product_translation (slug, language_code);
