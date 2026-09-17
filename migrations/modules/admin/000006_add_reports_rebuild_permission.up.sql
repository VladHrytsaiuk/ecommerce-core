INSERT INTO permissions (code, resource, action, description)
VALUES ('reports:rebuild', 'reports', 'rebuild', 'Rebuild reports CQRS projections')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code = 'reports:rebuild'
ON CONFLICT DO NOTHING;
