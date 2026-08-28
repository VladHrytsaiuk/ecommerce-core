-- A carrier refusal is not a financial cancellation: payment/refund and stock
-- effects remain exclusively in the dedicated order workflow. This terminal
-- operational status makes the exception visible to staff for reconciliation.
ALTER TABLE order_status_history
    DROP CONSTRAINT IF EXISTS order_status_history_actor_type_check;
ALTER TABLE order_status_history
    ADD CONSTRAINT order_status_history_actor_type_check
    CHECK (actor_type IN ('admin', 'system', 'payment_webhook', 'delivery_webhook', 'delivery_provider', 'customer'));

INSERT INTO order_status_definitions
    (code, name, description, color, sort_order, kind, is_initial, is_terminal, system_managed, customer_label)
VALUES
    ('delivery_refused', 'Delivery refused', 'Carrier reports refusal or return; financial settlement requires explicit review.', '#B45309', 70, 'fulfillment', FALSE, TRUE, TRUE, 'Delivery issue')
ON CONFLICT (code) DO NOTHING;

INSERT INTO order_status_transitions
    (from_status_code, to_status_code, allowed_triggers, requires_payment, requires_tracking_number, requires_reason, required_permission)
VALUES
    ('paid', 'shipped', ARRAY['delivery_webhook'], TRUE, TRUE, FALSE, ''),
    ('paid', 'delivery_refused', ARRAY['delivery_webhook'], TRUE, TRUE, FALSE, ''),
    ('processing', 'delivery_refused', ARRAY['delivery_webhook'], TRUE, TRUE, FALSE, ''),
    ('shipped', 'delivery_refused', ARRAY['delivery_webhook'], TRUE, TRUE, FALSE, ''),
    ('delivered', 'delivery_refused', ARRAY['delivery_webhook'], TRUE, TRUE, FALSE, ''),
    ('delivered', 'received', ARRAY['delivery_webhook'], TRUE, FALSE, FALSE, '')
ON CONFLICT (from_status_code, to_status_code) DO UPDATE
    SET allowed_triggers = EXCLUDED.allowed_triggers,
        requires_payment = EXCLUDED.requires_payment,
        requires_tracking_number = EXCLUDED.requires_tracking_number,
        requires_reason = EXCLUDED.requires_reason,
        required_permission = EXCLUDED.required_permission,
        updated_at = CURRENT_TIMESTAMP;
