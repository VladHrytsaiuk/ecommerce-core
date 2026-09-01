DROP INDEX IF EXISTS notification_jobs_due_idx;
DELETE FROM notification_jobs WHERE order_id IS NULL;
ALTER TABLE notification_jobs ALTER COLUMN order_id SET NOT NULL;
ALTER TABLE notification_jobs DROP COLUMN IF EXISTS next_retry_at, DROP COLUMN IF EXISTS retry_count,
    DROP COLUMN IF EXISTS payload, DROP COLUMN IF EXISTS recipient_email, DROP COLUMN IF EXISTS type;
