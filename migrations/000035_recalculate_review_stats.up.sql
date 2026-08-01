-- 000035_recalculate_review_stats.up.sql

UPDATE product p
SET 
  average_rating = COALESCE((
    SELECT AVG(rating) 
    FROM product_review 
    WHERE product_id = p.id AND is_approved = true AND parent_id IS NULL
  ), 0),
  reviews_count = COALESCE((
    SELECT COUNT(id) 
    FROM product_review 
    WHERE product_id = p.id AND is_approved = true AND parent_id IS NULL
  ), 0);
