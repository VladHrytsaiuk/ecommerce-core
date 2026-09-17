DROP TRIGGER IF EXISTS role_permissions_invalidate_cache ON role_permissions;
DROP TRIGGER IF EXISTS admin_user_roles_invalidate_cache ON admin_user_roles;
DROP FUNCTION IF EXISTS bump_authorization_version_on_role_permission();
DROP FUNCTION IF EXISTS bump_authorization_version_on_role_assignment();
DROP FUNCTION IF EXISTS bump_authorization_version_for_users(UUID[]);
