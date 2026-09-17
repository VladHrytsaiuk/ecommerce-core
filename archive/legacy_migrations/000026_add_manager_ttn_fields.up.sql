-- Manager token для secure доступу до замовлення менеджером
ALTER TABLE "order" ADD COLUMN manager_token_hash VARCHAR(64);
ALTER TABLE "order" ADD COLUMN manager_token_expires_at TIMESTAMPTZ;

-- TTN (товарно-транспортна накладна) дані від перевізника
ALTER TABLE "order" ADD COLUMN ttn_number VARCHAR(100);
ALTER TABLE "order" ADD COLUMN ttn_ref VARCHAR(100);
ALTER TABLE "order" ADD COLUMN ttn_created_at TIMESTAMPTZ;
ALTER TABLE "order" ADD COLUMN carrier_status VARCHAR(50);
ALTER TABLE "order" ADD COLUMN carrier_raw_response JSONB;

-- Індекс для пошуку по manager_token_hash (валідація при кожному запиті менеджера)
CREATE INDEX idx_order_manager_token_hash ON "order" (manager_token_hash) WHERE manager_token_hash IS NOT NULL;
