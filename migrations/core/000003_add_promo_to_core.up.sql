ALTER TABLE carts ADD COLUMN applied_promo_code VARCHAR(64);
ALTER TABLE carts ADD COLUMN discount_amount BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0);
ALTER TABLE orders ADD COLUMN applied_promo_code VARCHAR(64);
ALTER TABLE orders ADD COLUMN discount_amount BIGINT NOT NULL DEFAULT 0 CHECK (discount_amount >= 0);
ALTER TABLE orders ADD COLUMN promo_snapshot_type VARCHAR(16);
ALTER TABLE orders ADD COLUMN promo_snapshot_value BIGINT;
ALTER TABLE orders ADD COLUMN promo_snapshot_currency CHAR(3);
