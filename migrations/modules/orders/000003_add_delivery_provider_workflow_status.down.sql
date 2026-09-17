DELETE FROM order_status_transitions
WHERE (from_status_code, to_status_code) IN (
    ('paid', 'shipped'), ('paid', 'delivery_refused'),
    ('processing', 'delivery_refused'), ('shipped', 'delivery_refused'),
    ('delivered', 'delivery_refused')
);
DELETE FROM order_status_definitions WHERE code = 'delivery_refused';
ALTER TABLE order_status_history DROP CONSTRAINT IF EXISTS order_status_history_actor_type_check;
ALTER TABLE order_status_history
    ADD CONSTRAINT order_status_history_actor_type_check
    CHECK (actor_type IN ('admin', 'system', 'payment_webhook', 'delivery_webhook', 'customer'));

UPDATE order_status_transitions
SET allowed_triggers = ARRAY['admin', 'customer', 'system'], updated_at = CURRENT_TIMESTAMP
WHERE from_status_code = 'delivered' AND to_status_code = 'received';
