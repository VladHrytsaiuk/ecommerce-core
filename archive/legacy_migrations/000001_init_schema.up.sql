CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE language (
  code VARCHAR(2) PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE role (
  id SERIAL PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  description VARCHAR(255)
);

CREATE TABLE unit (
  id SERIAL PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  short_name VARCHAR(10) NOT NULL
);

CREATE TABLE order_status (
  id SERIAL PRIMARY KEY,
  name JSONB NOT NULL,
  code VARCHAR(50) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE badge (
  id SERIAL PRIMARY KEY,
  name JSONB NOT NULL,
  color_hex VARCHAR(10) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE category (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE brand (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name VARCHAR(100) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE attribute (
  id SERIAL PRIMARY KEY,
  sort_order INT NOT NULL DEFAULT 0,
  is_filterable BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE "user" (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  role_id INT NOT NULL REFERENCES role(id),
  first_name VARCHAR(100),
  last_name VARCHAR(100),
  password_hash VARCHAR(255),
  email VARCHAR(255),
  phone VARCHAR(20),
  is_email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  is_phone_verified BOOLEAN NOT NULL DEFAULT FALSE,
  wants_newsletter BOOLEAN NOT NULL DEFAULT FALSE,
  is_blocked BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE verify_code (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES "user"(id) ON DELETE CASCADE,
  target VARCHAR(255) NOT NULL,
  type VARCHAR(50) NOT NULL,
  code VARCHAR(10) NOT NULL,
  is_used BOOLEAN NOT NULL DEFAULT FALSE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_address (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  provider VARCHAR(50),
  delivery_type VARCHAR(50),
  full_address VARCHAR(255),
  city_ref VARCHAR(100),
  city_name VARCHAR(100),
  warehouse_ref VARCHAR(100),
  warehouse_name VARCHAR(255),
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE product (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  brand_id UUID REFERENCES brand(id),
  category_id UUID REFERENCES category(id),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE category_translation (
  category_id UUID NOT NULL REFERENCES category(id) ON DELETE CASCADE,
  language_code VARCHAR(2) NOT NULL REFERENCES language(code),
  name VARCHAR(100) NOT NULL,
  PRIMARY KEY (category_id, language_code)
);

CREATE TABLE product_translation (
  product_id UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  language_code VARCHAR(2) NOT NULL REFERENCES language(code),
  name VARCHAR(255) NOT NULL,
  description TEXT,
  usage_instructions TEXT,
  PRIMARY KEY (product_id, language_code)
);

CREATE TABLE product_badge (
  product_id UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  badge_id INT NOT NULL REFERENCES badge(id) ON DELETE CASCADE,
  PRIMARY KEY (product_id, badge_id)
);

CREATE TABLE product_variation (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  sku VARCHAR(100),
  barcode VARCHAR(50),
  price INT NOT NULL DEFAULT 0,
  stock_quantity INT NOT NULL DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE product_image (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  variation_id UUID REFERENCES product_variation(id) ON DELETE CASCADE,
  image_url VARCHAR(255) NOT NULL,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  is_hover BOOLEAN NOT NULL DEFAULT FALSE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE product_review (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES product(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  parent_id UUID REFERENCES product_review(id),
  rating NUMERIC(3, 2),
  comment TEXT,
  is_approved BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMPTZ
);

CREATE TABLE attribute_translation (
  attribute_id INT NOT NULL REFERENCES attribute(id) ON DELETE CASCADE,
  language_code VARCHAR(2) NOT NULL REFERENCES language(code),
  name VARCHAR(100) NOT NULL,
  PRIMARY KEY (attribute_id, language_code)
);

CREATE TABLE attribute_value (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  attribute_id INT NOT NULL REFERENCES attribute(id) ON DELETE CASCADE,
  value_string JSONB,
  value_numeric NUMERIC,
  unit_id INT REFERENCES unit(id),
  product_id UUID REFERENCES product(id) ON DELETE CASCADE,
  variation_id UUID REFERENCES product_variation(id) ON DELETE CASCADE,
  CONSTRAINT product_or_variation_check CHECK (
    (product_id IS NOT NULL AND variation_id IS NULL) OR 
    (product_id IS NULL AND variation_id IS NOT NULL)
  )
);

CREATE TABLE cart (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES "user"(id) ON DELETE CASCADE,
  session_id VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_or_session_check CHECK (
    (user_id IS NOT NULL AND session_id IS NULL) OR 
    (user_id IS NULL AND session_id IS NOT NULL)
  )
);

CREATE TABLE cart_item (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cart_id UUID NOT NULL REFERENCES cart(id) ON DELETE CASCADE,
  variation_id UUID NOT NULL REFERENCES product_variation(id) ON DELETE CASCADE,
  quantity INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE wishlist (
  user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  variation_id UUID NOT NULL REFERENCES product_variation(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, variation_id)
);

CREATE TABLE "order" (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
  status_id INT NOT NULL REFERENCES order_status(id),
  user_address_id UUID REFERENCES user_address(id) ON DELETE SET NULL,
  first_name VARCHAR(100),
  last_name VARCHAR(100),
  email VARCHAR(255),
  phone VARCHAR(20),
  total_price INT NOT NULL DEFAULT 0,
  admin_comment TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE order_item (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
  variation_id UUID NOT NULL REFERENCES product_variation(id) ON DELETE CASCADE,
  price INT NOT NULL DEFAULT 0,
  quantity INT NOT NULL DEFAULT 1,
  total_price INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE delivery (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
  provider VARCHAR(50),
  delivery_type VARCHAR(50),
  city_ref VARCHAR(100),
  city_name VARCHAR(100),
  warehouse_ref VARCHAR(100),
  warehouse_name VARCHAR(255),
  tracking_number VARCHAR(100),
  status VARCHAR(50),
  is_free_delivery BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE payment (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
  provider VARCHAR(50),
  transaction_id VARCHAR(255),
  amount INT NOT NULL DEFAULT 0,
  currency VARCHAR(10) NOT NULL DEFAULT 'UAH',
  status VARCHAR(50),
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
