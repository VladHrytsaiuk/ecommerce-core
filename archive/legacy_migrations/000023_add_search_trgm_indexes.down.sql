DROP INDEX IF EXISTS idx_product_variation_sku_trgm;
DROP INDEX IF EXISTS idx_product_translation_name_trgm;
-- We do NOT drop the extension pg_trgm as it might be used by other parts of the DB or manually installed by DBAs, 
-- or we can drop it safely if we know it's only used here. Let's drop it but with IF EXISTS.
-- DROP EXTENSION IF EXISTS pg_trgm; -- Commeting out to be safe, usually better not to drop extensions in down migrations.
