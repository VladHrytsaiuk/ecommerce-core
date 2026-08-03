ALTER TABLE orders ADD COLUMN cart_id UUID REFERENCES carts(id) ON DELETE SET NULL;
CREATE INDEX orders_cart_id_idx ON orders(cart_id) WHERE cart_id IS NOT NULL;

ALTER TABLE carts DROP CONSTRAINT carts_session_id_key;
CREATE UNIQUE INDEX carts_active_customer_unique ON carts(customer_id) WHERE status = 'active' AND customer_id IS NOT NULL;
CREATE UNIQUE INDEX carts_active_session_unique ON carts(session_id) WHERE status = 'active' AND session_id IS NOT NULL;
