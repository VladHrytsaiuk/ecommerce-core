-- wishlist is an optional module. It owns only its join table and references
-- stable Core/Catalog identities through foreign keys.
CREATE TABLE wishlist_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID,
    product_variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT wishlist_items_exactly_one_owner
        CHECK (num_nonnulls(user_id, session_id) = 1)
);

CREATE UNIQUE INDEX wishlist_items_user_variant_unique
    ON wishlist_items (user_id, product_variant_id)
    WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX wishlist_items_session_variant_unique
    ON wishlist_items (session_id, product_variant_id)
    WHERE session_id IS NOT NULL;

CREATE INDEX wishlist_items_user_created_at_idx
    ON wishlist_items (user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

CREATE INDEX wishlist_items_session_created_at_idx
    ON wishlist_items (session_id, created_at DESC)
    WHERE session_id IS NOT NULL;
