-- Відкат міграції session_id для wishlist

-- Видаляємо індекси
DROP INDEX IF EXISTS idx_wishlist_session_created;
DROP INDEX IF EXISTS idx_wishlist_session_variation;
DROP INDEX IF EXISTS idx_wishlist_user_variation;

-- Видаляємо CHECK constraint
ALTER TABLE wishlist DROP CONSTRAINT IF EXISTS wishlist_user_or_session_check;

-- Видаляємо колонку session_id
ALTER TABLE wishlist DROP COLUMN IF EXISTS session_id;

-- Повертаємо user_id як обов'язковий
ALTER TABLE wishlist ALTER COLUMN user_id SET NOT NULL;

-- Відновлюємо оригінальний PK
ALTER TABLE wishlist ADD PRIMARY KEY (user_id, variation_id);
