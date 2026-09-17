-- 000011_add_variation_old_price.down.sql
ALTER TABLE product_variation DROP COLUMN IF EXISTS old_price;
