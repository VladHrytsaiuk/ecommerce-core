-- Weight is a universal fulfilment fact. It belongs to the sellable variant
-- and is snapshotted on an order item so carrier retries never depend on a
-- product that may have changed after checkout.
ALTER TABLE product_variants
    ADD COLUMN weight_grams INTEGER NOT NULL DEFAULT 0
        CHECK (weight_grams >= 0);

ALTER TABLE order_items
    ADD COLUMN unit_weight_grams INTEGER NOT NULL DEFAULT 0
        CHECK (unit_weight_grams >= 0);
