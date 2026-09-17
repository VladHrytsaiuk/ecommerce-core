-- 000031_update_attributes_sort_order.up.sql

-- 1. Set all attributes to default high sort_order (500) so any custom attributes default to the end of the list
UPDATE attribute SET sort_order = 500;

-- 2. Update sort_order based on language-agnostic code
UPDATE attribute SET sort_order = 20 WHERE code = 'type';
UPDATE attribute SET sort_order = 30 WHERE code IN ('purpose', 'appointment', 'appointment_type');
UPDATE attribute SET sort_order = 40 WHERE code IN ('quantity', 'count', 'package_quantity', 'quantity_per_pack');
UPDATE attribute SET sort_order = 50 WHERE code IN ('weight', 'volume');
UPDATE attribute SET sort_order = 60 WHERE code IN ('composition', 'ingredients');
UPDATE attribute SET sort_order = 70 WHERE code IN ('packaging', 'pack');
UPDATE attribute SET sort_order = 80 WHERE code IN ('country', 'origin_country', 'origin');
UPDATE attribute SET sort_order = 90 WHERE code IN ('ean', 'barcode');

-- 3. Update sort_order based on Ukrainian translation names (to cover any legacy/custom attribute setups)
UPDATE attribute SET sort_order = 20 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) = 'тип');
UPDATE attribute SET sort_order = 30 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) IN ('призначення', 'призначення комплекту'));
UPDATE attribute SET sort_order = 40 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) IN ('кількість', 'кількість в упаковці'));
UPDATE attribute SET sort_order = 50 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) IN ('вага', 'об''єм'));
UPDATE attribute SET sort_order = 60 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) = 'склад');
UPDATE attribute SET sort_order = 70 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) = 'упаковка');
UPDATE attribute SET sort_order = 80 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) IN ('країна', 'країна виробник', 'країна-виробник'));
UPDATE attribute SET sort_order = 90 WHERE id IN (SELECT attribute_id FROM attribute_translation WHERE lower(name) IN ('ean', 'штрихкод', 'штрих-код'));
