-- Add SEO Meta tags to Category Translations
ALTER TABLE category_translation
    ADD COLUMN meta_title VARCHAR(255),
    ADD COLUMN meta_description TEXT,
    ADD COLUMN meta_keywords VARCHAR(255);

-- Add SEO Meta tags to Product Translations
ALTER TABLE product_translation
    ADD COLUMN meta_title VARCHAR(255),
    ADD COLUMN meta_description TEXT,
    ADD COLUMN meta_keywords VARCHAR(255);

-- Add Alt Text for Product Images (using JSONB for simple i18n mapping: {"uk": "...", "en": "..."})
ALTER TABLE product_image
    ADD COLUMN alt_text JSONB;
