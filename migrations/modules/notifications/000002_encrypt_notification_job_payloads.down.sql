ALTER TABLE notification_jobs DROP COLUMN IF EXISTS payload_ciphertext;
UPDATE notification_jobs SET recipient = 'legacy-redacted@invalid.local' WHERE recipient IS NULL;
ALTER TABLE notification_jobs ALTER COLUMN recipient SET NOT NULL;
