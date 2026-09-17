DROP INDEX IF EXISTS delivery_jobs_lease_reclaim_idx;
ALTER TABLE delivery_jobs DROP COLUMN last_failure_definite;
ALTER TABLE delivery_jobs DROP COLUMN lock_token;
