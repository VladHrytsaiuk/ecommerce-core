-- Rollback restores the former relational constraints. It intentionally fails
-- if logical references created while the forward migration was active are no
-- longer valid, preventing a silent loss of referential-integrity guarantees.
ALTER TABLE reviews
    ADD CONSTRAINT reviews_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    ADD CONSTRAINT reviews_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE product_review_ratings
    ADD CONSTRAINT product_review_ratings_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;
