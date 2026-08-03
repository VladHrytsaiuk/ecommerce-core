-- A checkout has one active payment provider. Its immutable provider reference
-- and commercial amount are recorded before any webhook can transition the
-- order. This makes provider callbacks verifiable rather than advisory.
CREATE UNIQUE INDEX payments_order_provider_unique
    ON payments(order_id, provider);
