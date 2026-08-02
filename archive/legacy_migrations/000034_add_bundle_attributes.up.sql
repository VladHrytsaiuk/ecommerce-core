-- 000034_add_bundle_attributes.up.sql

INSERT INTO language (code, name) VALUES ('uk', 'Ukrainian'), ('en', 'English') ON CONFLICT DO NOTHING;

INSERT INTO attribute (code, sort_order, is_filterable, unit_id) VALUES ('bundle_items_count', 10, false, 5);
INSERT INTO attribute (code, sort_order, is_filterable) VALUES ('bundle_items', 20, false);

INSERT INTO attribute_translation (attribute_id, language_code, name) 
VALUES ((SELECT id FROM attribute WHERE code = 'bundle_items_count'), 'uk', 'Кількість товарів у наборі'),
       ((SELECT id FROM attribute WHERE code = 'bundle_items_count'), 'en', 'Number of items in the set');

INSERT INTO attribute_translation (attribute_id, language_code, name) 
VALUES ((SELECT id FROM attribute WHERE code = 'bundle_items'), 'uk', 'Товари у наборі'),
       ((SELECT id FROM attribute WHERE code = 'bundle_items'), 'en', 'Items in the set');
