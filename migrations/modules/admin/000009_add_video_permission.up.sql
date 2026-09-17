-- Phase 25 shipped the permission-gated video upload endpoint but never seeded
-- the permission it checks, so Authorizer.Require could only ever deny it: the
-- admin video API was unreachable regardless of which role a user held.
INSERT INTO permissions (code, resource, action, description)
VALUES ('video:write', 'video', 'write', 'Upload videos and manage product video placements')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin' AND permissions.code = 'video:write'
ON CONFLICT DO NOTHING;
