-- Крок 1: Видаляємо старий PK (user_id, variation_id)
ALTER TABLE wishlist DROP CONSTRAINT wishlist_pkey;

-- Крок 2: Робимо user_id опціональним для підтримки анонімних сесій
ALTER TABLE wishlist ALTER COLUMN user_id DROP NOT NULL;

-- Крок 3: Додаємо session_id для анонімних юзерів
ALTER TABLE wishlist ADD COLUMN session_id VARCHAR(255);

-- Крок 4: Додаємо перевірку: або user_id, або session_id (не обидва одночасно)
ALTER TABLE wishlist ADD CONSTRAINT wishlist_user_or_session_check CHECK (
  (user_id IS NOT NULL AND session_id IS NULL) OR
  (user_id IS NULL AND session_id IS NOT NULL)
);

-- Крок 5: Унікальні індекси для запобігання дублікатів
CREATE UNIQUE INDEX idx_wishlist_user_variation ON wishlist (user_id, variation_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_wishlist_session_variation ON wishlist (session_id, variation_id) WHERE session_id IS NOT NULL;

-- Крок 6: Індекс для ефективного очищення застарілих анонімних записів
CREATE INDEX idx_wishlist_session_created ON wishlist (created_at) WHERE session_id IS NOT NULL;
