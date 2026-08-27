ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_code_format_check;

ALTER TABLE orders
    ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending_payment', 'paid', 'fulfillment_pending', 'shipped', 'delivered', 'cancelled', 'refunded'));
