-- Releasing expired reservations processed a whole batch in one transaction,
-- so a single reservation whose stock_items row no longer matched rolled the
-- batch back. Ordered by expires_at, that row returned first on every run and
-- the sweep stopped releasing anything at all: stock stayed reserved forever.
-- A reservation that cannot be reconciled is now quarantined instead, which
-- keeps the queue moving and makes the discrepancy visible.
ALTER TABLE inventory_reservations
    DROP CONSTRAINT inventory_reservations_status_check;

ALTER TABLE inventory_reservations
    ADD CONSTRAINT inventory_reservations_status_check
    CHECK (status IN ('active', 'released', 'committed', 'expired', 'release_failed'));

-- Operators alert on this being non-empty; it should never have rows.
CREATE INDEX inventory_reservations_release_failed_idx
    ON inventory_reservations (updated_at)
    WHERE status = 'release_failed';
