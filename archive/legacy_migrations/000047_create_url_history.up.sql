CREATE TABLE IF NOT EXISTS url_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type VARCHAR(50) NOT NULL, -- e.g., 'product', 'category', 'product_variation'
    entity_id UUID NOT NULL,
    old_slug VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_url_history_old_slug ON url_history(old_slug);
CREATE INDEX IF NOT EXISTS idx_url_history_entity ON url_history(entity_type, entity_id);
