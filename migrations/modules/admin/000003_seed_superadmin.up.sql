INSERT INTO roles (code, name, description, is_system)
VALUES ('super_admin', 'SuperAdmin', 'System bootstrap role with every registered admin permission', TRUE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (code, resource, action, description) VALUES
    ('catalog:write', 'catalog', 'write', 'Create and update catalog data'),
    ('promos:write', 'promos', 'write', 'Create and manage promotions'),
    ('orders:write', 'orders', 'write', 'Execute permitted order workflows'),
    ('reviews:write', 'reviews', 'write', 'Moderate reviews'),
    ('seo:write', 'seo', 'write', 'Manage SEO metadata'),
    ('badges:write', 'badges', 'write', 'Manage badges'),
    ('admin_users:read', 'admin_users', 'read', 'Read administrator accounts'),
    ('admin_users:write', 'admin_users', 'write', 'Manage administrator accounts')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role.id, permission.id
FROM roles AS role CROSS JOIN permissions AS permission
WHERE role.code = 'super_admin'
ON CONFLICT DO NOTHING;
