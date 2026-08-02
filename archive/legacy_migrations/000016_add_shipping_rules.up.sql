-- Таблиця правил безкоштовної доставки
-- provider = 'all' — глобальне правило для всіх провайдерів
-- provider = 'novaposhta' / 'ukrposhta' — per-provider override (на майбутнє)
CREATE TABLE IF NOT EXISTS shipping_rule (
    id            SERIAL PRIMARY KEY,
    provider      VARCHAR(50) NOT NULL DEFAULT 'all',
    min_order_amount INT NOT NULL DEFAULT 0,  -- мінімальна сума замовлення (в копійках) для безкоштовної доставки
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Унікальний індекс: один активний запис на кожного провайдера
CREATE UNIQUE INDEX idx_shipping_rule_provider ON shipping_rule (provider) WHERE is_active = true;

-- Дефолтне правило: безкоштовна доставка від 600 грн (60000 копійок)
INSERT INTO shipping_rule (provider, min_order_amount, is_active)
VALUES ('all', 60000, true);
