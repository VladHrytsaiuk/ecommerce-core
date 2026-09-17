ALTER TABLE category ADD COLUMN sort_order INT;

-- Тимчасово оновлюємо існуючі категорії унікальним значенням у межах батьківської категорії
WITH ordered_categories AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY parent_id ORDER BY created_at, id) - 1 AS seq
  FROM category
)
UPDATE category
SET sort_order = ordered_categories.seq
FROM ordered_categories
WHERE category.id = ordered_categories.id;

-- Встановлюємо NOT NULL та DEFAULT
ALTER TABLE category ALTER COLUMN sort_order SET NOT NULL;
ALTER TABLE category ALTER COLUMN sort_order SET DEFAULT 0;

-- Додаємо унікальні індекси
CREATE UNIQUE INDEX idx_category_parent_sort_order 
ON category (parent_id, sort_order) 
WHERE parent_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_category_root_sort_order 
ON category (sort_order) 
WHERE parent_id IS NULL AND deleted_at IS NULL;
