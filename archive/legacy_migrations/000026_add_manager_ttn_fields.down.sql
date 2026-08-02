DROP INDEX IF EXISTS idx_order_manager_token_hash;

ALTER TABLE "order" DROP COLUMN IF EXISTS carrier_raw_response;
ALTER TABLE "order" DROP COLUMN IF EXISTS carrier_status;
ALTER TABLE "order" DROP COLUMN IF EXISTS ttn_created_at;
ALTER TABLE "order" DROP COLUMN IF EXISTS ttn_ref;
ALTER TABLE "order" DROP COLUMN IF EXISTS ttn_number;
ALTER TABLE "order" DROP COLUMN IF EXISTS manager_token_expires_at;
ALTER TABLE "order" DROP COLUMN IF EXISTS manager_token_hash;
