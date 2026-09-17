-- Order workflow is a module-owned policy layer.  The initial migration is
-- deliberately additive: Core remains the owner of the orders aggregate and
-- later application migrations will enforce this registry when changing an
-- order's status.  There are no cross-module foreign keys to core.orders or
-- core.domain_events; UUIDs are stable integration identities.

CREATE TABLE order_status_definitions (
    code VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color VARCHAR(32) NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    kind VARCHAR(32) NOT NULL,
    is_initial BOOLEAN NOT NULL DEFAULT FALSE,
    is_terminal BOOLEAN NOT NULL DEFAULT FALSE,
    system_managed BOOLEAN NOT NULL DEFAULT FALSE,
    customer_label VARCHAR(128) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (btrim(code) <> ''),
    CHECK (btrim(name) <> ''),
    CHECK (kind IN ('payment', 'fulfillment', 'terminal', 'custom'))
);

-- PostgreSQL has no partial unique constraint, therefore this partial index
-- guarantees that an enabled workflow has only one entry status.
CREATE UNIQUE INDEX order_status_definitions_one_enabled_initial_idx
    ON order_status_definitions ((is_initial))
    WHERE is_initial AND enabled;
CREATE INDEX order_status_definitions_enabled_sort_idx
    ON order_status_definitions (sort_order, code)
    WHERE enabled;

CREATE TABLE order_status_transitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_status_code VARCHAR(64) NOT NULL,
    to_status_code VARCHAR(64) NOT NULL,
    allowed_triggers TEXT[] NOT NULL,
    requires_payment BOOLEAN NOT NULL DEFAULT FALSE,
    requires_tracking_number BOOLEAN NOT NULL DEFAULT FALSE,
    requires_reason BOOLEAN NOT NULL DEFAULT FALSE,
    required_permission VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT order_status_transitions_from_status_fkey
        FOREIGN KEY (from_status_code) REFERENCES order_status_definitions(code)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT order_status_transitions_to_status_fkey
        FOREIGN KEY (to_status_code) REFERENCES order_status_definitions(code)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT order_status_transitions_unique UNIQUE (from_status_code, to_status_code),
    CHECK (from_status_code <> to_status_code),
    CHECK (cardinality(allowed_triggers) > 0),
    CHECK (allowed_triggers <@ ARRAY['admin', 'system', 'payment_webhook', 'delivery_webhook', 'customer']::TEXT[])
);
CREATE INDEX order_status_transitions_from_status_idx
    ON order_status_transitions (from_status_code);

-- This is the immutable, domain-specific status journal.  It intentionally
-- contains no foreign key to core orders or Core Outbox: modules communicate
-- through stable UUIDs and must remain independently migratable.
CREATE TABLE order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    from_status_code VARCHAR(64),
    to_status_code VARCHAR(64) NOT NULL,
    actor_type VARCHAR(32) NOT NULL,
    actor_id UUID,
    reason TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    event_id UUID,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (actor_type IN ('admin', 'system', 'payment_webhook', 'delivery_webhook', 'customer')),
    CHECK (jsonb_typeof(metadata) = 'object')
);
CREATE UNIQUE INDEX order_status_history_event_id_unique_idx
    ON order_status_history (event_id)
    WHERE event_id IS NOT NULL;
CREATE INDEX order_status_history_order_occurred_idx
    ON order_status_history (order_id, occurred_at DESC, id DESC);

INSERT INTO order_status_definitions
    (code, name, description, color, sort_order, kind, is_initial, is_terminal, system_managed, customer_label)
VALUES
    ('pending_payment', 'Pending payment', 'Awaiting confirmed payment.', '#F59E0B', 10, 'payment', TRUE, FALSE, TRUE, 'Awaiting payment'),
    ('paid',            'Paid',            'Payment has been confirmed.', '#2563EB', 20, 'payment', FALSE, FALSE, TRUE, 'Paid'),
    -- Compatibility definition for orders created before the configurable
    -- workflow existed. It remains system-managed so historic orders can be
    -- fulfilled without granting the API a way to alter the legacy meaning.
    ('fulfillment_pending', 'Fulfillment pending', 'Legacy order awaiting fulfilment.', '#7C3AED', 25, 'fulfillment', FALSE, FALSE, TRUE, 'Processing'),
    ('processing',      'Processing',      'Order is being prepared for fulfilment.', '#7C3AED', 30, 'fulfillment', FALSE, FALSE, FALSE, 'Processing'),
    ('shipped',         'Shipped',         'Shipment has been handed to the carrier.', '#0891B2', 40, 'fulfillment', FALSE, FALSE, FALSE, 'Shipped'),
    ('delivered',       'Delivered',       'Carrier reports delivery.', '#059669', 50, 'fulfillment', FALSE, FALSE, FALSE, 'Delivered'),
    ('received',        'Received',        'Customer has received the order.', '#15803D', 60, 'terminal', FALSE, TRUE, FALSE, 'Received'),
    ('refunded',        'Refunded',        'Payment was refunded.', '#DC2626', 90, 'terminal', FALSE, TRUE, TRUE, 'Refunded'),
    ('cancelled',       'Cancelled',       'Order was cancelled before completion.', '#6B7280', 100, 'terminal', FALSE, TRUE, TRUE, 'Cancelled')
ON CONFLICT (code) DO NOTHING;

INSERT INTO order_status_transitions
    (from_status_code, to_status_code, allowed_triggers, requires_payment, requires_tracking_number, requires_reason, required_permission)
VALUES
    ('pending_payment', 'paid',      ARRAY['payment_webhook', 'system'], TRUE,  FALSE, FALSE, ''),
    ('pending_payment', 'cancelled', ARRAY['admin', 'system'],            FALSE, FALSE, FALSE, 'orders:write'),
    ('paid',            'processing',ARRAY['admin', 'system'],           TRUE,  FALSE, FALSE, 'orders:fulfillment:write'),
    ('paid',            'fulfillment_pending', ARRAY['admin', 'system'], TRUE, FALSE, FALSE, 'orders:fulfillment:write'),
    ('paid',            'refunded',  ARRAY['admin', 'payment_webhook'],  TRUE,  FALSE, TRUE,  'orders:refund:approve'),
    ('processing',      'shipped',   ARRAY['admin', 'delivery_webhook'], TRUE,  TRUE,  FALSE, 'orders:fulfillment:write'),
    ('fulfillment_pending', 'shipped', ARRAY['admin', 'delivery_webhook'], TRUE, TRUE, FALSE, 'orders:fulfillment:write'),
    ('processing',      'refunded',  ARRAY['admin', 'payment_webhook'],  TRUE,  FALSE, TRUE,  'orders:refund:approve'),
    ('shipped',         'delivered', ARRAY['admin', 'delivery_webhook'], TRUE,  FALSE, FALSE, 'orders:fulfillment:write'),
    ('delivered',       'received',  ARRAY['admin', 'customer', 'system'],TRUE, FALSE, FALSE, 'orders:fulfillment:write')
ON CONFLICT (from_status_code, to_status_code) DO NOTHING;
