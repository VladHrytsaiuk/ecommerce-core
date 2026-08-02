CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE locales (
    code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX locales_one_default ON locales (is_default) WHERE is_default;

CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX categories_parent_sort_idx ON categories(parent_id, sort_order);

CREATE TABLE category_translations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    locale VARCHAR(10) NOT NULL REFERENCES locales(code),
    name TEXT NOT NULL,
    description TEXT,
    slug VARCHAR(255) NOT NULL,
    UNIQUE (category_id, locale),
    UNIQUE (locale, slug)
);

CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX products_category_status_idx ON products(category_id, status);

CREATE TABLE product_translations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    locale VARCHAR(10) NOT NULL REFERENCES locales(code),
    name TEXT NOT NULL,
    description TEXT,
    slug VARCHAR(255) NOT NULL,
    UNIQUE (product_id, locale),
    UNIQUE (locale, slug)
);

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number VARCHAR(64) NOT NULL UNIQUE,
    customer_id UUID,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_payment'
        CHECK (status IN ('pending_payment', 'paid', 'fulfillment_pending', 'shipped', 'delivered', 'cancelled', 'refunded')),
    currency CHAR(3) NOT NULL,
    subtotal_amount BIGINT NOT NULL CHECK (subtotal_amount >= 0),
    tax_amount BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    total_amount BIGINT NOT NULL CHECK (total_amount >= 0),
    payment_provider VARCHAR(64),
    delivery_provider VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX orders_status_created_idx ON orders(status, created_at);
