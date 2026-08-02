-- =============================================================
-- Migration 000032: Revert add slug to product variation
-- =============================================================

DROP INDEX IF EXISTS idx_product_variation_slug;
ALTER TABLE product_variation DROP COLUMN IF EXISTS slug;
