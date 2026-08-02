DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='attribute' AND column_name='deleted_at') THEN
        ALTER TABLE attribute ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_attribute_deleted_at ON attribute(deleted_at);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='product' AND column_name='deleted_at') THEN
        ALTER TABLE product ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_product_deleted_at ON product(deleted_at);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='unit' AND column_name='deleted_at') THEN
        ALTER TABLE unit ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_unit_deleted_at ON unit(deleted_at);
    END IF;
END $$;
