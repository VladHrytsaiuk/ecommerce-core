DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'video:write');
DELETE FROM permissions WHERE code = 'video:write';
