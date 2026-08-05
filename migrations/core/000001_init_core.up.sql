CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE locales (
    code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX locales_one_default ON locales (is_default) WHERE is_default;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(320),
    phone VARCHAR(32),
    password_hash TEXT,
    role VARCHAR(32) NOT NULL DEFAULT 'customer'
        CHECK (role IN ('customer', 'manager', 'admin', 'owner')),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled', 'pending_verification')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX users_email_unique ON users(email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX users_phone_unique ON users(phone) WHERE phone IS NOT NULL;

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

CREATE TABLE product_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku VARCHAR(100) UNIQUE,
    barcode VARCHAR(100) UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'archived')),
    price_amount BIGINT NOT NULL CHECK (price_amount >= 0),
    currency CHAR(3) NOT NULL,
    weight_grams INTEGER NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'converted', 'abandoned')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (customer_id IS NOT NULL OR session_id IS NOT NULL)
);
CREATE UNIQUE INDEX carts_active_customer_unique ON carts(customer_id) WHERE status = 'active' AND customer_id IS NOT NULL;
CREATE UNIQUE INDEX carts_active_session_unique ON carts(session_id) WHERE status = 'active' AND session_id IS NOT NULL;

CREATE TABLE cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    UNIQUE (cart_id, variant_id)
);

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number VARCHAR(64) NOT NULL UNIQUE,
    customer_id UUID REFERENCES users(id) ON DELETE SET NULL,
    cart_id UUID REFERENCES carts(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_payment'
        CHECK (status IN ('pending_payment', 'paid', 'fulfillment_pending', 'shipped', 'delivered', 'cancelled', 'refunded')),
    currency CHAR(3) NOT NULL,
    subtotal_amount BIGINT NOT NULL CHECK (subtotal_amount >= 0),
    tax_amount BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    shipping_amount BIGINT NOT NULL DEFAULT 0 CHECK (shipping_amount >= 0),
    total_amount BIGINT NOT NULL CHECK (total_amount >= 0),
    payment_provider VARCHAR(64),
    delivery_provider VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX orders_status_created_idx ON orders(status, created_at);
CREATE INDEX orders_cart_id_idx ON orders(cart_id) WHERE cart_id IS NOT NULL;

CREATE TABLE order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES product_variants(id) ON DELETE SET NULL,
    product_name TEXT NOT NULL,
    sku VARCHAR(100),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_amount BIGINT NOT NULL CHECK (unit_price_amount >= 0),
    total_amount BIGINT NOT NULL CHECK (total_amount >= 0),
    currency CHAR(3) NOT NULL,
    unit_weight_grams INTEGER NOT NULL DEFAULT 0 CHECK (unit_weight_grams >= 0)
);

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    provider_reference VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'authorized', 'paid', 'failed', 'cancelled', 'refunded')),
    amount BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX payments_provider_reference_unique ON payments(provider, provider_reference) WHERE provider_reference IS NOT NULL;
CREATE UNIQUE INDEX payments_order_provider_unique ON payments(order_id, provider);

CREATE TABLE payment_checkout_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL, idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    amount BIGINT NOT NULL CHECK (amount >= 0), currency CHAR(3) NOT NULL, provider_reference VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'creating' CHECK (status IN ('creating', 'processing', 'created', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), last_error TEXT, locked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX payment_checkout_attempts_recovery_idx ON payment_checkout_attempts(status, updated_at) WHERE status = 'creating';
CREATE INDEX payment_checkout_attempts_processing_idx ON payment_checkout_attempts(status, locked_at) WHERE status = 'processing';

CREATE TABLE payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), provider VARCHAR(64) NOT NULL, event_id VARCHAR(255) NOT NULL,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE, provider_reference VARCHAR(255),
    event_status VARCHAR(32) NOT NULL CHECK (event_status IN ('pending', 'paid', 'failed')), amount BIGINT NOT NULL CHECK (amount >= 0), currency CHAR(3) NOT NULL,
    processing_status VARCHAR(32) NOT NULL DEFAULT 'processing' CHECK (processing_status IN ('processing', 'processed')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, processed_at TIMESTAMPTZ, UNIQUE (provider, event_id)
);
CREATE INDEX payment_webhook_events_order_received_idx ON payment_webhook_events(order_id, received_at);

CREATE TABLE deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    tracking_number VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'created', 'in_transit', 'delivered', 'failed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX deliveries_provider_tracking_unique ON deliveries(provider, tracking_number) WHERE tracking_number IS NOT NULL;

CREATE TABLE order_delivery_details (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE, recipient_name TEXT NOT NULL, recipient_phone VARCHAR(32) NOT NULL,
    country_code CHAR(2), postal_code VARCHAR(32), city TEXT, line1 TEXT, line2 TEXT, locality_id VARCHAR(255), service_point_id VARCHAR(255), created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE delivery_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE, provider VARCHAR(64) NOT NULL, idempotency_key UUID NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'retrying', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, locked_at TIMESTAMPTZ, last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX delivery_jobs_ready_idx ON delivery_jobs(status, available_at) WHERE status IN ('pending', 'retrying');
