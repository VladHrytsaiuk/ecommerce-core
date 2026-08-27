DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN ('orders:fulfillment:write', 'orders:workflow:read', 'orders:workflow:write')
);

DELETE FROM permissions
WHERE code IN ('orders:fulfillment:write', 'orders:workflow:read', 'orders:workflow:write');
