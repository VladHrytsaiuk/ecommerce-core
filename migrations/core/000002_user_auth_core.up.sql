-- users is defined in the immutable clean-slate Core baseline
-- (000001_init_core). This migration strengthens its universal identity
-- invariants and adds OAuth identities without placing provider data on users.

ALTER TABLE users
    ADD CONSTRAINT users_identity_present
        CHECK (email IS NOT NULL OR phone IS NOT NULL),
    ADD CONSTRAINT users_email_not_blank
        CHECK (email IS NULL OR btrim(email) <> ''),
    ADD CONSTRAINT users_phone_not_blank
        CHECK (phone IS NULL OR btrim(phone) <> '');

-- Authentication normalizes email case before lookup. Mirror that invariant in
-- PostgreSQL so concurrent registrations cannot create case-only duplicates.
DROP INDEX users_email_unique;
CREATE UNIQUE INDEX users_email_unique
    ON users (lower(email))
    WHERE email IS NOT NULL;

-- OAuth provider credentials and tokens are never stored here. This table
-- records only the stable, verified external subject linked to a Core user.
CREATE TABLE user_oauth_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    provider_email VARCHAR(320),
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT user_oauth_identities_provider_valid
        CHECK (provider ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    CONSTRAINT user_oauth_identities_subject_not_blank
        CHECK (btrim(subject) <> ''),
    CONSTRAINT user_oauth_identities_email_not_blank
        CHECK (provider_email IS NULL OR btrim(provider_email) <> ''),
    CONSTRAINT user_oauth_identities_provider_subject_unique
        UNIQUE (provider, subject)
);

CREATE INDEX user_oauth_identities_user_id_idx
    ON user_oauth_identities (user_id);

-- One-time OAuth authorization state. state_hash prevents state disclosure if
-- the database is read; consumed_at makes callbacks replay-safe across API
-- instances.
CREATE TABLE oauth_authorization_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(64) NOT NULL,
    state_hash BYTEA NOT NULL UNIQUE,
    redirect_uri TEXT NOT NULL,
    nonce TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT oauth_authorization_attempts_provider_valid
        CHECK (provider ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    CONSTRAINT oauth_authorization_attempts_redirect_uri_not_blank
        CHECK (btrim(redirect_uri) <> ''),
    CONSTRAINT oauth_authorization_attempts_nonce_not_blank
        CHECK (btrim(nonce) <> ''),
    CONSTRAINT oauth_authorization_attempts_code_verifier_not_blank
        CHECK (btrim(code_verifier) <> '')
);

CREATE INDEX oauth_authorization_attempts_expiry_idx
    ON oauth_authorization_attempts (expires_at)
    WHERE consumed_at IS NULL;
