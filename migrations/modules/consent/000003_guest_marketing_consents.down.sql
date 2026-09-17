DROP INDEX IF EXISTS customer_consents_active_marketing_email_idx;
ALTER TABLE customer_consents DROP CONSTRAINT IF EXISTS customer_consents_subject_check;
ALTER TABLE customer_consents DROP COLUMN IF EXISTS contact_email;
ALTER TABLE customer_consents ALTER COLUMN customer_id SET NOT NULL;
