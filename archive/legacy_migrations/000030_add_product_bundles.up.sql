-- 000030_add_product_bundles.up.sql

-- 1. Add bundle columns to product table
ALTER TABLE product ADD COLUMN IF NOT EXISTS is_bundle BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE product ADD COLUMN IF NOT EXISTS price_strategy VARCHAR(20) NOT NULL DEFAULT 'manual';

-- 2. Create junction table for bundle components
CREATE TABLE IF NOT EXISTS product_bundle_item (
  bundle_id    UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  variation_id UUID NOT NULL REFERENCES product_variation(id) ON DELETE CASCADE,
  quantity     INT  NOT NULL DEFAULT 1 CHECK (quantity > 0),
  PRIMARY KEY (bundle_id, variation_id)
);

-- 3. Performance indexes
-- Reverse lookup: which bundles contain a given variation
CREATE INDEX IF NOT EXISTS idx_product_bundle_item_variation_id ON product_bundle_item (variation_id);

-- Partial indexes for active-state filtering (used in FindAll NOT EXISTS subquery)
CREATE INDEX IF NOT EXISTS idx_product_is_bundle ON product (is_bundle) WHERE is_bundle = true;
CREATE INDEX IF NOT EXISTS idx_product_is_active_not_deleted ON product (id) WHERE is_active = true AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_product_variation_is_active_not_deleted ON product_variation (id, product_id) WHERE is_active = true AND deleted_at IS NULL;
