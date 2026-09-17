-- This rollback destroys data and cannot restore it.
--
-- The up made order_id nullable so the scheduled dispatcher could queue mail
-- that belongs to no order. Restoring NOT NULL means those rows cannot stay, so
-- the DELETE below removes every scheduled notification job: support replies,
-- abandoned-cart reminders, back-in-stock notices and return-status mail, sent
-- or still queued. An unsent message deleted here is never sent.
--
-- There is no way to keep them and restore the constraint. If the queue matters
-- more than the rollback, copy it out first:
--
--   CREATE TABLE notification_jobs_rescued AS
--   SELECT * FROM notification_jobs WHERE order_id IS NULL;
DROP INDEX IF EXISTS notification_jobs_due_idx;
DELETE FROM notification_jobs WHERE order_id IS NULL;
ALTER TABLE notification_jobs ALTER COLUMN order_id SET NOT NULL;
ALTER TABLE notification_jobs DROP COLUMN IF EXISTS next_retry_at, DROP COLUMN IF EXISTS retry_count,
    DROP COLUMN IF EXISTS payload, DROP COLUMN IF EXISTS recipient_email, DROP COLUMN IF EXISTS type;
