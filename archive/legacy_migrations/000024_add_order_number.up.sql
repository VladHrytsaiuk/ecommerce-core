-- Sequence для генерації номерів замовлень (починається з 10000)
CREATE SEQUENCE IF NOT EXISTS order_number_seq START WITH 10000;

-- Додаємо колонку order_number з автоматичним значенням
ALTER TABLE "order" ADD COLUMN order_number BIGINT NOT NULL DEFAULT nextval('order_number_seq');

-- Унікальний індекс для номеру замовлення
CREATE UNIQUE INDEX idx_order_number ON "order" (order_number);
