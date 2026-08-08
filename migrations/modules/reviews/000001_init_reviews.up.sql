-- reviews is an optional module. It owns review moderation and its aggregate
-- projection; Core products receive no module-specific columns.
CREATE TABLE reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL CHECK (char_length(comment) BETWEEN 1 AND 5000),
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT reviews_one_per_user_product UNIQUE (user_id, product_id)
);

CREATE INDEX reviews_approved_product_created_at_idx
    ON reviews (product_id, created_at DESC)
    WHERE status = 'approved';

CREATE INDEX reviews_moderation_status_created_at_idx
    ON reviews (status, created_at ASC)
    WHERE status = 'pending';

-- This is a module-owned read projection for Catalog. average_rating_hundredths
-- avoids floating point values in domain and database calculations.
CREATE TABLE product_review_ratings (
    product_id UUID PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    review_count INTEGER NOT NULL CHECK (review_count > 0),
    rating_sum INTEGER NOT NULL CHECK (rating_sum >= review_count),
    average_rating_hundredths SMALLINT NOT NULL CHECK (average_rating_hundredths BETWEEN 100 AND 500),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
