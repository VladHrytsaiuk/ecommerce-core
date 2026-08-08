-- comparison is an optional engagement module. A list is scoped to one
-- category, while an owner may keep independent lists for multiple categories.
CREATE TABLE comparison_lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT comparison_lists_exactly_one_owner
        CHECK (num_nonnulls(user_id, session_id) = 1)
);

CREATE UNIQUE INDEX comparison_lists_user_category_unique
    ON comparison_lists (user_id, category_id)
    WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX comparison_lists_session_category_unique
    ON comparison_lists (session_id, category_id)
    WHERE session_id IS NOT NULL;

CREATE INDEX comparison_lists_user_updated_at_idx
    ON comparison_lists (user_id, updated_at DESC)
    WHERE user_id IS NOT NULL;

CREATE INDEX comparison_lists_session_updated_at_idx
    ON comparison_lists (session_id, updated_at DESC)
    WHERE session_id IS NOT NULL;

CREATE TABLE comparison_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    comparison_list_id UUID NOT NULL REFERENCES comparison_lists(id) ON DELETE CASCADE,
    product_variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT comparison_items_list_variant_unique
        UNIQUE (comparison_list_id, product_variant_id)
);

CREATE INDEX comparison_items_list_created_at_idx
    ON comparison_items (comparison_list_id, created_at DESC);
