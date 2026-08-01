-- =============================================================
-- Migration 000032: Add slug to product variation (nullable)
-- =============================================================

ALTER TABLE product_variation ADD COLUMN slug VARCHAR(255);

-- Backfill existing variations
-- We try to use the parent product's slug combined with the SKU.
-- If SKU is empty, we fall back to the first 8 characters of the variation UUID.
UPDATE product_variation pv
SET slug = COALESCE(
    (SELECT p.slug FROM product p WHERE p.id = pv.product_id) || '-' || LOWER(pv.sku),
    LEFT(pv.id::TEXT, 8)
);

-- Note: We keep the column nullable so that direct database inserts in tests
-- (which do not specify a slug) do not fail, but we enforce uniqueness.
CREATE UNIQUE INDEX idx_product_variation_slug ON product_variation (slug) WHERE deleted_at IS NULL AND slug IS NOT NULL;
