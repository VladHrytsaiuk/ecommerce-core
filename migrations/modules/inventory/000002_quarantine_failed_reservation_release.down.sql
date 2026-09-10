DROP INDEX IF EXISTS inventory_reservations_release_failed_idx;

-- Quarantined rows have no representation in the previous vocabulary. Treat
-- them as expired: their stock was never returned, which is what 'expired'
-- meant before this migration introduced the distinction.
UPDATE inventory_reservations SET status = 'expired' WHERE status = 'release_failed';

ALTER TABLE inventory_reservations
    DROP CONSTRAINT inventory_reservations_status_check;

ALTER TABLE inventory_reservations
    ADD CONSTRAINT inventory_reservations_status_check
    CHECK (status IN ('active', 'released', 'committed', 'expired'));
