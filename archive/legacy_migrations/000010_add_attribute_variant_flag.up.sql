ALTER TABLE attribute ADD COLUMN is_variant_specific BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN attribute.is_variant_specific IS 'TRUE if attribute value depends on product variation, FALSE if global for entire product';
