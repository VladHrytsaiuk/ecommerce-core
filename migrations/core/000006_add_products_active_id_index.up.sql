-- Search reindexing traverses active products through an ID keyset cursor.
-- This partial index avoids scanning inactive catalog rows on every batch.
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_active_id_idx
    ON products (id)
    WHERE status = 'active';
