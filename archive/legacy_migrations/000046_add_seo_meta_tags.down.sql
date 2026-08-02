-- Remove Alt Text from Product Images
ALTER TABLE product_image
    DROP COLUMN IF EXISTS alt_text;

-- Remove SEO Meta tags from Product Translations
ALTER TABLE product_translation
    DROP COLUMN IF EXISTS meta_title,
    DROP COLUMN IF EXISTS meta_description,
    DROP COLUMN IF EXISTS meta_keywords;

-- Remove SEO Meta tags from Category Translations
ALTER TABLE category_translation
    DROP COLUMN IF EXISTS meta_title,
    DROP COLUMN IF EXISTS meta_description,
    DROP COLUMN IF EXISTS meta_keywords;
