CREATE TABLE product_media (
 product_id UUID NOT NULL,
 variant_id UUID,
 asset_id UUID NOT NULL,
 role VARCHAR(16) NOT NULL CHECK (role IN ('MAIN','HOVER','GALLERY')),
 position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
 PRIMARY KEY (product_id, asset_id, role),
 UNIQUE (product_id, variant_id, role, position)
);
CREATE INDEX product_media_product_position_idx ON product_media(product_id, role, position);
