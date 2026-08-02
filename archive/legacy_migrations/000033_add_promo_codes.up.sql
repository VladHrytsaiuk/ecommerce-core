CREATE TABLE promo_code (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    discount_type VARCHAR(20) NOT NULL, -- 'percentage', 'fixed'
    discount_value INT NOT NULL, -- значення у відсотках або копійках
    is_active BOOLEAN NOT NULL DEFAULT true,
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    usage_limit INT,
    usage_count INT NOT NULL DEFAULT 0,
    usage_limit_per_user INT DEFAULT 1,
    min_order_subtotal INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE promo_code_category (
    promo_code_id UUID REFERENCES promo_code(id) ON DELETE CASCADE,
    category_id UUID REFERENCES category(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_code_id, category_id)
);

CREATE TABLE promo_code_brand (
    promo_code_id UUID REFERENCES promo_code(id) ON DELETE CASCADE,
    brand_id UUID REFERENCES brand(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_code_id, brand_id)
);

CREATE TABLE promo_code_product (
    promo_code_id UUID REFERENCES promo_code(id) ON DELETE CASCADE,
    product_id UUID REFERENCES product(id) ON DELETE CASCADE,
    PRIMARY KEY (promo_code_id, product_id)
);

CREATE TABLE promo_code_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_code_id UUID REFERENCES promo_code(id) ON DELETE CASCADE,
    order_id UUID NOT NULL,
    user_id UUID,
    email VARCHAR(255),
    phone VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE cart ADD COLUMN promo_code_id UUID REFERENCES promo_code(id) ON DELETE SET NULL;
ALTER TABLE "order" ADD COLUMN promo_code VARCHAR(50);
ALTER TABLE "order" ADD COLUMN discount_amount INT NOT NULL DEFAULT 0;
ALTER TABLE order_item ADD COLUMN discount_amount INT NOT NULL DEFAULT 0;
ALTER TABLE order_item ADD COLUMN final_total_price INT NOT NULL DEFAULT 0;
