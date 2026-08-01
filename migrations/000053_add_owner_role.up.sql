INSERT INTO role (id, name, description)
VALUES (3, 'owner', 'Store owner with administrator management access')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description;

UPDATE "user"
SET role_id = 3, updated_at = CURRENT_TIMESTAMP
WHERE LOWER(email) = LOWER('oleksii13work@gmail.com')
  AND deleted_at IS NULL;

SELECT setval(pg_get_serial_sequence('role', 'id'), GREATEST((SELECT MAX(id) FROM role), 3));
