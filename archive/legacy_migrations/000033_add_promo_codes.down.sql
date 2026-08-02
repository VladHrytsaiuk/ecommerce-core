ALTER TABLE order_item DROP COLUMN IF EXISTS final_total_price;
ALTER TABLE order_item DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE "order" DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE "order" DROP COLUMN IF EXISTS promo_code;
ALTER TABLE cart DROP COLUMN IF EXISTS promo_code_id;

DROP TABLE IF EXISTS promo_code_usage CASCADE;
DROP TABLE IF EXISTS promo_code_product CASCADE;
DROP TABLE IF EXISTS promo_code_brand CASCADE;
DROP TABLE IF EXISTS promo_code_category CASCADE;
DROP TABLE IF EXISTS promo_code CASCADE;
