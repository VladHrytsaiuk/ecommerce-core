-- Product options are Catalog-owned. product_variants remains the immutable
-- sellable/inventory unit; this extension only defines its option matrix.
CREATE TABLE product_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT product_options_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT product_options_id_product_unique UNIQUE (id, product_id)
);

-- Option codes/names are case-insensitive within one product. This prevents
-- distinct "Color" and "color" axes from being introduced by separate admin
-- sessions.
CREATE UNIQUE INDEX product_options_product_name_unique
    ON product_options (product_id, lower(btrim(name)));
CREATE INDEX product_options_product_position_idx
    ON product_options (product_id, position, id);

CREATE TABLE product_option_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    option_id UUID NOT NULL,
    product_id UUID NOT NULL,
    value VARCHAR(255) NOT NULL,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT product_option_values_value_not_blank CHECK (btrim(value) <> ''),
    CONSTRAINT product_option_values_option_product_fkey
        FOREIGN KEY (option_id, product_id)
        REFERENCES product_options(id, product_id) ON DELETE CASCADE,
    CONSTRAINT product_option_values_id_option_product_unique
        UNIQUE (id, option_id, product_id),
    CONSTRAINT product_option_values_id_product_unique UNIQUE (id, product_id)
);

CREATE UNIQUE INDEX product_option_values_option_value_unique
    ON product_option_values (option_id, lower(btrim(value)));
CREATE INDEX product_option_values_option_position_idx
    ON product_option_values (option_id, position, id);

-- The composite FK below proves that both the variant and option value belong
-- to the same product. It is deliberately backed by a unique index instead
-- of changing the existing core primary key.
CREATE UNIQUE INDEX product_variants_id_product_unique
    ON product_variants (id, product_id);

CREATE TABLE variant_option_values (
    variant_id UUID NOT NULL,
    option_value_id UUID NOT NULL,
    option_id UUID NOT NULL,
    product_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (variant_id, option_value_id),
    CONSTRAINT variant_option_values_one_value_per_option_unique
        UNIQUE (variant_id, option_id),
    CONSTRAINT variant_option_values_variant_product_fkey
        FOREIGN KEY (variant_id, product_id)
        REFERENCES product_variants(id, product_id) ON DELETE CASCADE,
    CONSTRAINT variant_option_values_value_option_product_fkey
        FOREIGN KEY (option_value_id, option_id, product_id)
        REFERENCES product_option_values(id, option_id, product_id) ON DELETE CASCADE
);
CREATE INDEX variant_option_values_product_variant_idx
    ON variant_option_values (product_id, variant_id);

-- A normal unique index cannot express uniqueness of a set of rows. The
-- deferred constraint runs at COMMIT, after a facade has attached all option
-- values, and rejects two variants of the same configurable product with the
-- same canonical (sorted UUID) option-value set. It also makes a product with
-- configured options reject two unconfigured variants.
CREATE OR REPLACE FUNCTION catalog_enforce_unique_variant_option_combination()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_product_id UUID := COALESCE(NEW.product_id, OLD.product_id);
    duplicate_exists BOOLEAN;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM product_options WHERE product_id = target_product_id) THEN
        RETURN NULL;
    END IF;

    SELECT EXISTS (
        SELECT 1
        FROM (
            SELECT variants.id,
                   COALESCE(
                       array_agg(links.option_value_id ORDER BY links.option_value_id)
                           FILTER (WHERE links.option_value_id IS NOT NULL),
                       ARRAY[]::UUID[]
                   ) AS combination
            FROM product_variants AS variants
            LEFT JOIN variant_option_values AS links ON links.variant_id = variants.id
            WHERE variants.product_id = target_product_id
            GROUP BY variants.id
        ) AS combinations
        GROUP BY combination
        HAVING COUNT(*) > 1
    ) INTO duplicate_exists;

    IF duplicate_exists THEN
        RAISE EXCEPTION 'duplicate variant option-value combination for product %', target_product_id
            USING ERRCODE = 'unique_violation';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER variant_option_values_combination_unique
AFTER INSERT OR UPDATE OR DELETE ON variant_option_values
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION catalog_enforce_unique_variant_option_combination();

-- Creating an option must remain possible for legacy products that already
-- have simple variants. Variant creation is covered, while the join trigger
-- enforces every actual option-value combination at COMMIT.
CREATE CONSTRAINT TRIGGER product_variants_combination_unique
AFTER INSERT OR UPDATE OR DELETE ON product_variants
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION catalog_enforce_unique_variant_option_combination();
