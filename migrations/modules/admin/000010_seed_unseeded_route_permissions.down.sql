DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN ('legal:write', 'privacy:write', 'support:read', 'support:write')
);
DELETE FROM permissions
WHERE code IN ('legal:write', 'privacy:write', 'support:read', 'support:write');
