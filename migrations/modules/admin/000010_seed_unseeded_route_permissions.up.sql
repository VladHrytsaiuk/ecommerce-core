-- The consent and support modules gate eight admin routes on four permissions
-- that were never seeded, so Authorizer.Require could only ever deny them:
-- publishing legal documents, listing and approving GDPR privacy requests, and
-- the entire support agent surface were unreachable regardless of which role a
-- user held — including super_admin, since a permission with no row cannot be
-- in anyone's granted set.
--
-- This is the third time this has shipped; video:write was the second. A test
-- now asserts that every permission constant a route guards has a row here, so
-- there should not be a fourth.
INSERT INTO permissions (code, resource, action, description)
VALUES
    ('legal:write',   'legal',   'write', 'Create and publish versioned legal documents'),
    ('privacy:write', 'privacy', 'write', 'Review and approve GDPR privacy requests'),
    ('support:read',  'support', 'read',  'Read customer support tickets'),
    ('support:write', 'support', 'write', 'Reply to and transition customer support tickets')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('legal:write', 'privacy:write', 'support:read', 'support:write')
ON CONFLICT DO NOTHING;
