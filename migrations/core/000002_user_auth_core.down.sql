DROP TABLE IF EXISTS oauth_authorization_attempts;
DROP TABLE IF EXISTS user_oauth_identities;

DROP INDEX IF EXISTS users_email_unique;
CREATE UNIQUE INDEX users_email_unique
    ON users (email)
    WHERE email IS NOT NULL;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_phone_not_blank,
    DROP CONSTRAINT IF EXISTS users_email_not_blank,
    DROP CONSTRAINT IF EXISTS users_identity_present;
