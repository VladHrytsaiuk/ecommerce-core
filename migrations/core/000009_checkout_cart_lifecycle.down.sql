DROP INDEX IF EXISTS carts_active_session_unique;
DROP INDEX IF EXISTS carts_active_customer_unique;
ALTER TABLE carts ADD CONSTRAINT carts_session_id_key UNIQUE (session_id);
DROP INDEX IF EXISTS orders_cart_id_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS cart_id;
