-- Refresh tokens keep a customer signed in past ACCESS_TOKEN_DURATION.
--
-- Access tokens carry the role and cannot be revoked, so they are capped at an
-- hour, fifteen minutes by default. With nothing to renew them a customer was
-- signed out every fifteen minutes — mid-checkout included — while
-- REFRESH_TOKEN_DURATION sat in .env.example promising a week.
--
-- A refresh token is an opaque random value; only its SHA-256 is stored, so a
-- copy of this table authenticates nobody. Every refresh consumes the token and
-- issues a new one in the same family. A token presented a second time means two
-- parties hold it, and the whole family is revoked. Every token in a family
-- shares the family's expires_at: refreshing keeps a sign-in alive but never
-- extends it, so a stolen family cannot outlive the sign-in it was taken from.
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id UUID NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT refresh_tokens_hash_is_sha256 CHECK (octet_length(token_hash) = 32)
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);
-- Revoking a family, on reuse or on logout, touches every token in it.
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
-- The purge removes rows past expires_at in bounded batches, oldest first.
CREATE INDEX refresh_tokens_expires_idx ON refresh_tokens (expires_at);
