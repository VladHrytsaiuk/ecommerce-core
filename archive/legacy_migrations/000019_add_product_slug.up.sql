-- =============================================================
-- Migration 000018: SEO slugs + language-agnostic filter codes
-- =============================================================

-- Part 1: Product slugs
ALTER TABLE product ADD COLUMN slug VARCHAR(255);

-- Backfill existing products with a temporary slug (uuid prefix guarantees uniqueness)
UPDATE product p
SET slug = LEFT(p.id::TEXT, 8)
WHERE p.slug IS NULL;

-- Enforce constraints
ALTER TABLE product ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX idx_product_slug ON product (slug) WHERE deleted_at IS NULL;

-- Part 2: Attribute value codes (language-agnostic filter keys)
ALTER TABLE attribute_value ADD COLUMN value_code VARCHAR(100);
CREATE INDEX idx_attribute_value_code ON attribute_value (attribute_id, value_code);
