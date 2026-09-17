DROP INDEX IF EXISTS deliveries_due_idx;
ALTER TABLE deliveries DROP COLUMN IF EXISTS next_check_at;
