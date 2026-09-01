-- Returns / RMA owns the lifecycle of a return request.  The order, customer
-- and variant UUIDs are stable integration identities, not database foreign
-- keys: Orders, Identity and Catalog remain independently migratable modules.

CREATE TABLE return_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'new',
    refund_mode VARCHAR(32) NOT NULL DEFAULT 'full',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('new', 'approved', 'rejected', 'received', 'refunded', 'closed')),
    CHECK (refund_mode IN ('full', 'partial', 'store_credit'))
);

CREATE INDEX return_requests_order_created_idx
    ON return_requests (order_id, created_at DESC, id DESC);
CREATE INDEX return_requests_customer_created_idx
    ON return_requests (customer_id, created_at DESC, id DESC);
CREATE INDEX return_requests_status_updated_idx
    ON return_requests (status, updated_at ASC, id ASC);

CREATE TABLE return_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_request_id UUID NOT NULL,
    variant_id UUID NOT NULL,
    quantity INTEGER NOT NULL,
    condition VARCHAR(32) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (quantity > 0),
    CHECK (condition IN ('unopened', 'opened', 'damaged')),
    CONSTRAINT return_items_request_variant_unique UNIQUE (return_request_id, variant_id)
);

CREATE INDEX return_items_request_idx ON return_items (return_request_id, id);
CREATE INDEX return_items_variant_idx ON return_items (variant_id);

CREATE TABLE return_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_request_id UUID NOT NULL,
    status VARCHAR(32) NOT NULL,
    actor_type VARCHAR(32) NOT NULL,
    actor_id UUID,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('new', 'approved', 'rejected', 'received', 'refunded', 'closed')),
    CHECK (actor_type IN ('system', 'admin', 'customer')),
    CHECK (
        (actor_type = 'system' AND actor_id IS NULL)
        OR (actor_type IN ('admin', 'customer') AND actor_id IS NOT NULL)
    )
);

CREATE INDEX return_status_history_request_created_idx
    ON return_status_history (return_request_id, created_at DESC, id DESC);
