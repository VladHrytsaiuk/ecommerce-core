-- Two workers shared this table and one destroyed the other's work.
--
-- OrderPaidEventHandler enqueues a receipt with its render data encrypted into
-- payload_ciphertext, leaving the plaintext payload column NULL, and claims the
-- row by id. DurableWorker scans for due rows with no filter at all, so it
-- claimed those receipts too, found payload NULL, failed to parse it and
-- recorded the job dead. The handler's next delivery saw status 'dead', treated
-- it as already settled and acknowledged the outbox entry: the order
-- confirmation was never sent and nothing was left to retry.
--
-- The same path poisoned every retry. A failed send returns the job to
-- 'pending', which is exactly what DurableWorker scans for, so a single
-- transient SMTP error was enough to lose the receipt permanently.
--
-- dispatcher states which worker owns a row. provider was carrying this by
-- accident — the scheduler wrote the literal 'scheduled' into a column meant
-- for an email provider — which is not something a claim query should depend on.
ALTER TABLE notification_jobs ADD COLUMN dispatcher VARCHAR(16);

UPDATE notification_jobs
   SET dispatcher = CASE WHEN provider = 'scheduled' THEN 'scheduler' ELSE 'outbox' END
 WHERE dispatcher IS NULL;

ALTER TABLE notification_jobs ALTER COLUMN dispatcher SET NOT NULL;

-- Deliberately no default: an insert that does not name an owner must fail
-- loudly rather than land in whichever worker's queue the default points at.
ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_dispatcher_check
    CHECK (dispatcher IN ('outbox', 'scheduler'));

-- The shape invariant that broke, now enforced by the database rather than by
-- each worker remembering to filter: a scheduler job carries a plaintext
-- payload to render from, an outbox job carries ciphertext and no plaintext.
ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_dispatcher_payload_check
    CHECK ((dispatcher = 'scheduler' AND payload IS NOT NULL)
        OR (dispatcher = 'outbox' AND payload IS NULL));

-- Both claim indexes were partial on status alone, so once the claim query
-- also filters on dispatcher they would have stopped covering it and every
-- claim would have read the other worker's rows to discard them. They are
-- rebuilt here rather than in a later migration.
--
-- Only the scheduler scans. Order-paid jobs are claimed by primary key from
-- the outbox delivery that owns them and need no index of their own.
DROP INDEX notification_jobs_due_idx;
DROP INDEX notification_jobs_claim_idx;

CREATE INDEX notification_jobs_scheduler_due_idx
    ON notification_jobs (next_retry_at, id)
    WHERE dispatcher = 'scheduler' AND status IN ('pending', 'failed');

CREATE INDEX notification_jobs_scheduler_reclaim_idx
    ON notification_jobs (locked_at, id)
    WHERE dispatcher = 'scheduler' AND status = 'sending';

-- Receipts a deployment already lost to this bug cannot be recovered here:
-- their outbox delivery was acknowledged, so nothing remains to re-drive them.
-- They are identifiable for manual resend — DurableWorker killed a job without
-- ever incrementing attempts, which a genuinely exhausted receipt has at the
-- handler's maximum:
--
--   SELECT id, order_id, created_at FROM notification_jobs
--    WHERE dispatcher = 'outbox' AND status = 'dead' AND attempts = 0;
