DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'reports:rebuild');
DELETE FROM permissions WHERE code = 'reports:rebuild';
