-- Two leases in this system still had nothing to fence them, and both follow
-- the pattern already corrected for event_deliveries, sync_outbox,
-- notification_jobs and delivery_jobs: a claim that can be taken over after a
-- timeout, with terminal writes that match on status alone.
--
-- payment_webhook_events is the costlier of the two. A replica whose lease
-- expired while it was still running could call Abandon, which deletes on
-- status = 'processing' — deleting the row a newer replica currently holds.
-- The provider's next retry then found no deduplication record and the
-- callback was processed again. Money was never at risk: MarkPaid re-checks
-- provider, amount, currency and provider_reference against both the order and
-- the registered payment attempt under FOR UPDATE. The cost was duplicated
-- work, misleading log entries, and a lost deduplication record.
ALTER TABLE payment_webhook_events ADD COLUMN lock_token UUID;

-- Rows already in flight hold no token. The takeover path issues one as soon
-- as their lease expires, so nothing needs backfilling.
