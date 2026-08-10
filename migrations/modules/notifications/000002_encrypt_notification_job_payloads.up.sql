-- New notification jobs persist render data only as authenticated ciphertext.
-- The nullable legacy recipient column is retained for a forward-compatible
-- rollout; all Phase-8 writers leave it NULL.
ALTER TABLE notification_jobs ADD COLUMN payload_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_jobs ALTER COLUMN recipient DROP NOT NULL;
ALTER TABLE notification_jobs ALTER COLUMN payload_ciphertext DROP DEFAULT;
