DROP INDEX IF EXISTS idx_cart_session_updated;
CREATE INDEX idx_cart_session_created ON cart (created_at) WHERE session_id IS NOT NULL;
