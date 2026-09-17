-- 000031_update_attributes_sort_order.down.sql

-- Reset all attributes to default 0
UPDATE attribute SET sort_order = 0;
