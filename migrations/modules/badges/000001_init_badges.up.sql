-- badges is an optional module. Display names are localized in a normalized
-- child table; slug is a stable, non-localized machine identifier.
CREATE TABLE badges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(120) NOT NULL UNIQUE,
    color VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT badges_slug_not_blank CHECK (btrim(slug) <> '')
);

CREATE TABLE badge_translations (
    badge_id UUID NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    locale VARCHAR(10) NOT NULL,
    name VARCHAR(120) NOT NULL,
    PRIMARY KEY (badge_id, locale),
    CONSTRAINT badge_translations_name_not_blank CHECK (btrim(name) <> '')
);

CREATE TABLE product_badges (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    badge_id UUID NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (product_id, badge_id)
);

CREATE INDEX product_badges_badge_id_idx ON product_badges (badge_id);
