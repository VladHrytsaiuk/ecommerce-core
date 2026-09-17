-- A restock operation is idempotent per returned line.  It is deliberately
-- module-owned and has no FK to inventory: the application adapter invokes
-- Inventory through its narrow port in the same local PostgreSQL transaction.
CREATE TABLE return_restock_operations (
    return_item_id UUID PRIMARY KEY,
    return_request_id UUID NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX return_restock_operations_request_idx
    ON return_restock_operations (return_request_id, completed_at DESC);
