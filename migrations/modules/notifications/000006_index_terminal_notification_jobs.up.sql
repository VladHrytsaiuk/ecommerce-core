-- Retention had no index to work with. Every index on this table is partial on
-- the statuses the dispatcher claims — pending, sending, failed — which is
-- right for the dispatcher and useless for the purge, so both retention queries
-- read the whole table: a parallel sequential scan plus a top-N sort to find a
-- thousand candidates among two hundred thousand rows.
--
-- These are the same shape on the cold side.
--
-- The candidate index leads with updated_at because the purge orders by it and
-- stops at its batch size: leading with status instead would satisfy the filter
-- and then need a sort over everything that matched. Both terminal statuses are
-- inside the partial predicate, so the per-row status test is a cheap filter on
-- a scan that already stops early.
CREATE INDEX notification_jobs_terminal_idx
    ON notification_jobs (updated_at, id)
    WHERE status IN ('sent', 'dead');

-- Dead jobs are counted once per retention cycle for the gauge an operator
-- alerts on. They should be a vanishing fraction of the table, so this index is
-- tiny, and it keeps that count off the far larger set of sent rows.
CREATE INDEX notification_jobs_dead_idx
    ON notification_jobs (updated_at)
    WHERE status = 'dead';
