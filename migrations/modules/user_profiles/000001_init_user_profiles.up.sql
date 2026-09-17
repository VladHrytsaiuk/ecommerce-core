-- user_profiles is an optional module-owned 1:1 extension. Flexible profile
-- attributes do not leak into Core users and are validated by ProfilePolicy in
-- the application layer.
CREATE TABLE user_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    schema_version INTEGER NOT NULL DEFAULT 1
        CHECK (schema_version > 0),
    revision INTEGER NOT NULL DEFAULT 1
        CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT user_profiles_attributes_object
        CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE INDEX user_profiles_attributes_gin_idx
    ON user_profiles USING GIN (attributes jsonb_path_ops);
