-- The permission cache is keyed on admin_users.authorization_version, so a
-- change only takes effect once that number moves. Two CLI commands bumped it,
-- and both of them only ever grant. There is no API for role management, so a
-- revocation is made directly in SQL — and nothing there touched the version.
--
-- The result was a five-minute window, the cache TTL, during which a revoked
-- role or a permission removed from a role stayed in force. Deactivating an
-- admin outright was never affected: is_active is read on every call.
--
-- A trigger rather than a convention. There is no code path to add the bump
-- to, and "remember to increment it" is not something a schema change made in
-- psql at 2am will honour.

CREATE OR REPLACE FUNCTION bump_authorization_version_for_users(target UUID[])
RETURNS VOID AS $$
    UPDATE admin_users
       SET authorization_version = authorization_version + 1,
           updated_at = CURRENT_TIMESTAMP
     WHERE user_id = ANY(target);
$$ LANGUAGE sql;

-- Granting or revoking a role affects exactly one admin.
CREATE OR REPLACE FUNCTION bump_authorization_version_on_role_assignment()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM bump_authorization_version_for_users(ARRAY[COALESCE(NEW.user_id, OLD.user_id)]);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER admin_user_roles_invalidate_cache
    AFTER INSERT OR UPDATE OR DELETE ON admin_user_roles
    FOR EACH ROW EXECUTE FUNCTION bump_authorization_version_on_role_assignment();

-- Changing a role's permissions affects everyone holding that role.
CREATE OR REPLACE FUNCTION bump_authorization_version_on_role_permission()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM bump_authorization_version_for_users(
        ARRAY(SELECT user_id FROM admin_user_roles WHERE role_id = COALESCE(NEW.role_id, OLD.role_id)));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER role_permissions_invalidate_cache
    AFTER INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION bump_authorization_version_on_role_permission();
