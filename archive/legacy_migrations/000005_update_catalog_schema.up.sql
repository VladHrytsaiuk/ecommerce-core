ALTER TABLE category ADD COLUMN parent_id UUID REFERENCES category(id);

ALTER TABLE product ADD COLUMN average_rating NUMERIC(3, 2) NOT NULL DEFAULT 0;
ALTER TABLE product ADD COLUMN reviews_count INT NOT NULL DEFAULT 0;
ALTER TABLE product ADD COLUMN deleted_at TIMESTAMPTZ;

ALTER TABLE product_review ALTER COLUMN rating TYPE SMALLINT USING rating::SMALLINT;
ALTER TABLE product_variation DROP COLUMN IF EXISTS stock_quantity;

CREATE INDEX idx_product_variation_price_active ON product_variation (product_id, is_active, price);
