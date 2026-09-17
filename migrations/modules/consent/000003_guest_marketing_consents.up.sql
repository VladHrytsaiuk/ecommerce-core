ALTER TABLE customer_consents ALTER COLUMN customer_id DROP NOT NULL;
ALTER TABLE customer_consents ADD COLUMN contact_email VARCHAR(320);
ALTER TABLE customer_consents ADD CONSTRAINT customer_consents_subject_check CHECK (
    customer_id IS NOT NULL OR (document_type = 'marketing' AND contact_email IS NOT NULL)
);
CREATE UNIQUE INDEX customer_consents_active_marketing_email_idx
    ON customer_consents (contact_email)
    WHERE document_type = 'marketing' AND withdrawn_at IS NULL;
