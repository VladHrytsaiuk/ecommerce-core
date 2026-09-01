-- Typed customer data and reusable address book. These records are module
-- owned; orders preserve their own immutable delivery snapshots.
CREATE TABLE customer_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    date_of_birth DATE,
    gender VARCHAR(32),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT customer_profiles_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT customer_profiles_gender_not_blank CHECK (gender IS NULL OR btrim(gender) <> '')
);

CREATE TABLE customer_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(100) NOT NULL,
    country VARCHAR(2) NOT NULL,
    city VARCHAR(120) NOT NULL,
    line1 VARCHAR(255) NOT NULL,
    line2 VARCHAR(255),
    zip_code VARCHAR(32) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT customer_addresses_title_not_blank CHECK (btrim(title) <> ''),
    CONSTRAINT customer_addresses_country_format CHECK (country ~ '^[A-Z]{2}$'),
    CONSTRAINT customer_addresses_city_not_blank CHECK (btrim(city) <> ''),
    CONSTRAINT customer_addresses_line1_not_blank CHECK (btrim(line1) <> ''),
    CONSTRAINT customer_addresses_zip_not_blank CHECK (btrim(zip_code) <> '')
);

CREATE INDEX customer_addresses_user_id_idx ON customer_addresses (user_id, created_at DESC);
CREATE UNIQUE INDEX customer_addresses_one_default_per_user
    ON customer_addresses (user_id) WHERE is_default;
