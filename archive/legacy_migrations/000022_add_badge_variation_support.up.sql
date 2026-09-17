-- Step 1: Drop the old composite primary key
ALTER TABLE product_badge DROP CONSTRAINT product_badge_pkey;

-- Step 2: Make product_id nullable
ALTER TABLE product_badge ALTER COLUMN product_id DROP NOT NULL;

-- Step 3: Add variation_id column
ALTER TABLE product_badge ADD COLUMN variation_id UUID REFERENCES product_variation(id) ON DELETE CASCADE;

-- Step 4: Add sort_order to badge table for display priority
ALTER TABLE badge ADD COLUMN sort_order INT NOT NULL DEFAULT 0;

-- Step 5: Add check constraint — either product or variation, not both
ALTER TABLE product_badge ADD CONSTRAINT badge_product_or_variation_check CHECK (
  (product_id IS NOT NULL AND variation_id IS NULL) OR
  (product_id IS NULL AND variation_id IS NOT NULL)
);

-- Step 6: Add unique index to prevent duplicate badge assignments
CREATE UNIQUE INDEX idx_product_badge_unique
  ON product_badge (badge_id, COALESCE(product_id, '00000000-0000-0000-0000-000000000000'), COALESCE(variation_id, '00000000-0000-0000-0000-000000000000'));
