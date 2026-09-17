-- The order workflow module now owns the directed status graph. Core keeps a
-- conservative syntactic guard only; semantic validation happens through the
-- module policy inside the same transaction as the order update.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;

ALTER TABLE orders
    ADD CONSTRAINT orders_status_code_format_check
    CHECK (status ~ '^[a-z][a-z0-9_]{0,63}$') NOT VALID;
