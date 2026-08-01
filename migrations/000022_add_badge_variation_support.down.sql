-- Reverse: drop unique index
DROP INDEX IF EXISTS idx_product_badge_unique;

-- Reverse: drop check constraint
ALTER TABLE product_badge DROP CONSTRAINT IF EXISTS badge_product_or_variation_check;

-- Reverse: drop sort_order from badge
ALTER TABLE badge DROP COLUMN IF EXISTS sort_order;

-- Reverse: drop variation_id column
ALTER TABLE product_badge DROP COLUMN IF EXISTS variation_id;

-- Reverse: make product_id NOT NULL again
ALTER TABLE product_badge ALTER COLUMN product_id SET NOT NULL;

-- Reverse: restore original primary key
ALTER TABLE product_badge ADD PRIMARY KEY (product_id, badge_id);
