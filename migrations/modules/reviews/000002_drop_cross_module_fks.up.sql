-- Reviews refers to Catalog and Identity aggregates by stable UUID only.
-- Cross-module validity is enforced through ports/events, not database FKs.
ALTER TABLE reviews
    DROP CONSTRAINT IF EXISTS reviews_product_id_fkey,
    DROP CONSTRAINT IF EXISTS reviews_user_id_fkey;

ALTER TABLE product_review_ratings
    DROP CONSTRAINT IF EXISTS product_review_ratings_product_id_fkey;
