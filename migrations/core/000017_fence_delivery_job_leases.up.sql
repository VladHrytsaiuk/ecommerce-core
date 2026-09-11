-- delivery_jobs had a lease (locked_at) with nothing to fence it.
--
-- The reclaim path returns any job whose lease expired to 'retrying'. Nothing
-- stopped the worker that still held it: it had not crashed, only taken longer
-- than five minutes on a carrier call. A second worker then claimed the same
-- job and issued a second waybill, and the first worker's Complete still
-- matched on `status = 'processing'` — so it overwrote the live worker's
-- shipment with its own, or moved the job to failed while the second worker
-- was mid-flight. The carrier had two shipments and the store had one record
-- of one of them.
--
-- lock_token identifies the claim, not the row. Every terminal write now has
-- to present the token it was given, so a worker whose lease was reclaimed
-- discovers that on its first write instead of corrupting a newer claim. This
-- is the same fencing already applied to event_deliveries and sync_outbox.
ALTER TABLE delivery_jobs ADD COLUMN lock_token UUID;

-- Jobs already in flight during this deployment have no token. The reclaim
-- sweep returns them to 'retrying' after their lease expires, at which point
-- they are claimed with one.
CREATE INDEX delivery_jobs_lease_reclaim_idx
    ON delivery_jobs (locked_at)
    WHERE status = 'processing';

-- Re-dispatching after an ambiguous failure is what actually prints a second
-- waybill: a timeout is indistinguishable from a success whose response was
-- lost. An adapter that knows its request was rejected before reaching the
-- carrier says so, and that attempt is safe to retry. Anything else is not,
-- and a carrier with no lookup capability cannot be asked which it was.
--
-- FALSE is the safe default: an unrecorded failure is treated as possibly
-- having created a shipment.
ALTER TABLE delivery_jobs
    ADD COLUMN last_failure_definite BOOLEAN NOT NULL DEFAULT FALSE;
