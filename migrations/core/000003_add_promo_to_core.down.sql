ALTER TABLE orders DROP COLUMN IF EXISTS promo_snapshot_currency;
ALTER TABLE orders DROP COLUMN IF EXISTS promo_snapshot_value;
ALTER TABLE orders DROP COLUMN IF EXISTS promo_snapshot_type;
ALTER TABLE orders DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE orders DROP COLUMN IF EXISTS applied_promo_code;
ALTER TABLE carts DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE carts DROP COLUMN IF EXISTS applied_promo_code;
