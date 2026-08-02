DROP INDEX IF EXISTS idx_product_variation_price_active;
ALTER TABLE product_review ALTER COLUMN rating TYPE NUMERIC(3, 2) USING rating::NUMERIC(3, 2);
ALTER TABLE product_variation ADD COLUMN stock_quantity INT NOT NULL DEFAULT 0;

ALTER TABLE product DROP COLUMN deleted_at;
ALTER TABLE product DROP COLUMN reviews_count;
ALTER TABLE product DROP COLUMN average_rating;

ALTER TABLE category DROP COLUMN parent_id;
