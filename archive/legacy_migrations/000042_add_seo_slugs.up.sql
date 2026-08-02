ALTER TABLE category ADD COLUMN slug VARCHAR(255);

UPDATE category c
SET slug = LEFT(c.id::TEXT, 8)
WHERE c.slug IS NULL;

ALTER TABLE category ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX idx_category_slug ON category (slug) WHERE deleted_at IS NULL;


ALTER TABLE brand ADD COLUMN slug VARCHAR(255);

UPDATE brand b
SET slug = LEFT(b.id::TEXT, 8)
WHERE b.slug IS NULL;

ALTER TABLE brand ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX idx_brand_slug ON brand (slug) WHERE deleted_at IS NULL;
