DELETE FROM role_permissions WHERE role_id = (SELECT id FROM roles WHERE code = 'super_admin');
DELETE FROM roles WHERE code = 'super_admin';
