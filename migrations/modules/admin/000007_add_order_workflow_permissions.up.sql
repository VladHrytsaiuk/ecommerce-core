INSERT INTO permissions (code, resource, action, description)
VALUES
    ('orders:fulfillment:write', 'orders', 'fulfillment_write', 'Move orders through operational fulfilment statuses'),
    ('orders:workflow:read', 'orders', 'workflow_read', 'Read order status workflow configuration and history'),
    ('orders:workflow:write', 'orders', 'workflow_write', 'Manage custom order status definitions and transitions')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id
FROM roles
CROSS JOIN permissions
WHERE roles.code = 'super_admin'
  AND permissions.code IN ('orders:fulfillment:write', 'orders:workflow:read', 'orders:workflow:write')
ON CONFLICT DO NOTHING;
