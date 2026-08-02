-- 000052_update_review_status.up.sql

ALTER TABLE product_review 
  ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'pending',
  ADD COLUMN reject_reason TEXT,
  ADD CONSTRAINT chk_product_review_status CHECK (status IN ('pending', 'approved', 'rejected'));

UPDATE product_review SET status = 'approved' WHERE is_approved = true;

ALTER TABLE product_review DROP COLUMN is_approved;

CREATE INDEX idx_product_review_status ON product_review(status);
