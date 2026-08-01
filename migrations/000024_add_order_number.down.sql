-- Видаляємо індекс та колонку order_number
DROP INDEX IF EXISTS idx_order_number;
ALTER TABLE "order" DROP COLUMN IF EXISTS order_number;

-- Видаляємо sequence
DROP SEQUENCE IF EXISTS order_number_seq;
