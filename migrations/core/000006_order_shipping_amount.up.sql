ALTER TABLE orders
    ADD COLUMN shipping_amount BIGINT NOT NULL DEFAULT 0
        CHECK (shipping_amount >= 0);
