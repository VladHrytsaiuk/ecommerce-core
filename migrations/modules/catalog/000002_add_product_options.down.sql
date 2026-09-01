DROP TRIGGER IF EXISTS variant_option_values_combination_unique ON variant_option_values;
DROP TRIGGER IF EXISTS product_variants_combination_unique ON product_variants;
DROP FUNCTION IF EXISTS catalog_enforce_unique_variant_option_combination();
DROP TABLE IF EXISTS variant_option_values;
DROP INDEX IF EXISTS product_variants_id_product_unique;
DROP TABLE IF EXISTS product_option_values;
DROP TABLE IF EXISTS product_options;
