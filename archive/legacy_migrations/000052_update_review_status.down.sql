-- 000052_update_review_status.down.sql

ALTER TABLE product_review 
  ADD COLUMN is_approved BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE product_review SET is_approved = true WHERE status = 'approved';

DROP INDEX IF EXISTS idx_product_review_status;

ALTER TABLE product_review 
  DROP CONSTRAINT IF EXISTS chk_product_review_status,
  DROP COLUMN status,
  DROP COLUMN reject_reason;
