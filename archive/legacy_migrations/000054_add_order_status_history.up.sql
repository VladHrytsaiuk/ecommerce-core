-- 1. Таблиця історії статусів
CREATE TABLE order_status_history (
    id             UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    order_id       UUID NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    from_status_id INT REFERENCES order_status(id),
    to_status_id   INT NOT NULL REFERENCES order_status(id),
    source         VARCHAR(50) NOT NULL,
    admin_user_id  UUID REFERENCES "user"(id),
    comment        TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_osh_order_id ON order_status_history(order_id);
CREATE INDEX idx_osh_created_at ON order_status_history(created_at);

-- 2. Seed історії для існуючих замовлень (лише де ще немає записів)
INSERT INTO order_status_history (order_id, from_status_id, to_status_id, source, comment)
SELECT o.id, NULL, o.status_id, 'system', 'Стан на момент запуску історії'
FROM "order" o
WHERE NOT EXISTS (
    SELECT 1 FROM order_status_history h WHERE h.order_id = o.id
);

-- 3. Виправлення старих замовлень: Processing + TTN → Shipped (UPDATE...RETURNING)
WITH corrected AS (
    UPDATE "order"
    SET status_id = 4, updated_at = CURRENT_TIMESTAMP
    WHERE ttn_number IS NOT NULL
      AND ttn_number <> ''
      AND status_id = 3
    RETURNING id
)
INSERT INTO order_status_history (
    order_id, from_status_id, to_status_id, source, comment
)
SELECT id, 3, 4, 'system', 'Міграція: TTN існує → статус виправлено на Shipped'
FROM corrected;
