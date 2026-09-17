-- Крок 1: Унікальний індекс — один кошик на юзера
CREATE UNIQUE INDEX idx_cart_user ON cart (user_id) WHERE user_id IS NOT NULL;

-- Крок 2: Унікальний індекс — один кошик на сесію
CREATE UNIQUE INDEX idx_cart_session ON cart (session_id) WHERE session_id IS NOT NULL;

-- Крок 3: Унікальний індекс — один запис варіації на кошик
CREATE UNIQUE INDEX idx_cart_item_cart_variation ON cart_item (cart_id, variation_id);

-- Крок 4: Індекс для ефективного очищення застарілих анонімних кошиків
CREATE INDEX idx_cart_session_created ON cart (created_at) WHERE session_id IS NOT NULL;
