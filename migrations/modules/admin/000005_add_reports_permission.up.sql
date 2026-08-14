INSERT INTO permissions (code, resource, action, description)
VALUES ('reports:read', 'reports', 'read', 'Read aggregated business analytics')
ON CONFLICT (code) DO NOTHING;

-- Ensure the Reports reader is assigned to the canonical seeded SuperAdmin
-- role. The media permission has its own migration and lifecycle.
INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code = 'reports:read'
ON CONFLICT DO NOTHING;
