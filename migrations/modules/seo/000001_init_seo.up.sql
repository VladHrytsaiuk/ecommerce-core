-- seo is an optional module. Its polymorphic resource identity deliberately
-- avoids adding SEO columns or resource-specific foreign keys to Core tables.
CREATE TABLE seo_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(64) NOT NULL,
    resource_id UUID NOT NULL,
    locale VARCHAR(10) NOT NULL,
    title VARCHAR(255),
    description VARCHAR(500),
    keywords TEXT,
    og_image_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT seo_metadata_resource_type_not_blank CHECK (btrim(resource_type) <> ''),
    CONSTRAINT seo_metadata_resource_locale_unique UNIQUE (resource_type, resource_id, locale)
);

CREATE INDEX seo_metadata_resource_lookup_idx
    ON seo_metadata (resource_type, resource_id, locale);
