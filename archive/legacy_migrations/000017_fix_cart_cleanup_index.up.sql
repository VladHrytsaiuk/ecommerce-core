-- Замінюємо індекс для очищення анонімних кошиків: використовуємо updated_at замість created_at,
-- щоб активні кошики (до яких нещодавно додавали товари) не видалялися передчасно.
DROP INDEX IF EXISTS idx_cart_session_created;
CREATE INDEX idx_cart_session_updated ON cart (updated_at) WHERE session_id IS NOT NULL;
