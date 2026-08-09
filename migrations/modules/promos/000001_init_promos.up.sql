CREATE TABLE promocodes (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), code VARCHAR(64) NOT NULL UNIQUE,
 discount_type VARCHAR(16) NOT NULL CHECK(discount_type IN ('percent','fixed')),
 discount_value BIGINT NOT NULL CHECK(discount_value > 0), currency CHAR(3),
 is_active BOOLEAN NOT NULL DEFAULT TRUE, valid_until TIMESTAMPTZ,
 usage_limit INTEGER, usage_count INTEGER NOT NULL DEFAULT 0 CHECK(usage_count >= 0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK ((discount_type = 'percent' AND discount_value <= 10000 AND currency IS NULL) OR (discount_type = 'fixed' AND currency IS NOT NULL)),
 CHECK (usage_limit IS NULL OR usage_limit >= 0)
);
CREATE TABLE promo_redemptions (
 promo_id UUID NOT NULL REFERENCES promocodes(id) ON DELETE RESTRICT,
 order_id UUID NOT NULL UNIQUE,
 status VARCHAR(16) NOT NULL CHECK(status IN ('reserved','committed','released')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX promo_redemptions_promo_status_idx ON promo_redemptions(promo_id,status);
