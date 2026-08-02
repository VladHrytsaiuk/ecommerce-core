CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_product_translation_name_trgm ON product_translation USING gin (name gin_trgm_ops);
CREATE INDEX idx_product_variation_sku_trgm ON product_variation USING gin (sku gin_trgm_ops);
