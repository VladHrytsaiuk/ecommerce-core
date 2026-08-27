-- Each order item stores its immutable share of the order promotion. The
-- allocation is produced in Checkout with deterministic largest-remainder
-- arithmetic, so item discounts always sum exactly to orders.discount_amount.
ALTER TABLE order_items
    ADD COLUMN discount_amount BIGINT NOT NULL DEFAULT 0
        CHECK (discount_amount >= 0 AND discount_amount <= total_amount);
