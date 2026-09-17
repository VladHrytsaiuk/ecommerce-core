-- Опціональна локалізована назва конкретної варіації (напр. різна кількість штук у назві).
-- Порожня → у відповіді підставляється назва товару (fallback).
ALTER TABLE product_variation ADD COLUMN IF NOT EXISTS name JSONB;
