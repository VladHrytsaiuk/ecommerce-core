INSERT INTO permissions (code, resource, action, description)
VALUES ('media:write', 'media', 'write', 'Upload and manage media assets')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'media:write'
ON CONFLICT DO NOTHING;
