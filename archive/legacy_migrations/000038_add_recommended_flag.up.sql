ALTER TABLE product ADD COLUMN is_recommended BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_product_is_recommended ON product(is_recommended) WHERE is_recommended = TRUE;
