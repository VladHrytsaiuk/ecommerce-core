-- Seed-дані для order_status (довідникова таблиця з JSONB перекладами)
INSERT INTO order_status (id, code, name, sort_order) VALUES
  (1, 'pending_payment', '{"uk":"Очікує оплати","en":"Pending Payment"}', 1),
  (2, 'paid',            '{"uk":"Оплачено","en":"Paid"}', 2),
  (3, 'processing',      '{"uk":"В обробці","en":"Processing"}', 3),
  (4, 'shipped',         '{"uk":"Відправлено","en":"Shipped"}', 4),
  (5, 'delivered',       '{"uk":"Доставлено","en":"Delivered"}', 5),
  (6, 'cancelled',       '{"uk":"Скасовано","en":"Cancelled"}', 6),
  (7, 'refunded',        '{"uk":"Повернено","en":"Refunded"}', 7)
ON CONFLICT (id) DO NOTHING;

SELECT setval('order_status_id_seq', 7);
