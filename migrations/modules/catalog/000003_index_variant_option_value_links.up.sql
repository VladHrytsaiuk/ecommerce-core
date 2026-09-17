-- variant_option_values references product_option_values on
-- (option_value_id, option_id, product_id), and PostgreSQL checks the
-- referencing side of a foreign key without an index unless one leads with the
-- first referencing column. The primary key here is (variant_id,
-- option_value_id), which leads with the wrong one.
--
-- Deleting a product cascades through product_options and product_option_values
-- into this table, so an admin deleting one product sequentially scanned the
-- whole variant-to-option join table once per option value it owned.
CREATE INDEX variant_option_values_option_value_idx
    ON variant_option_values (option_value_id);
