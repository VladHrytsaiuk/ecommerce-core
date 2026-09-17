INSERT INTO permissions (code, resource, action, description)
VALUES ('returns:write', 'returns', 'write', 'Approve and receive return requests')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'returns:write'
ON CONFLICT DO NOTHING;
