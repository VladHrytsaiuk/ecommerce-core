-- Verification state belongs to the Identity-owned users aggregate. Checkout
-- reads it only through a narrow port wired in Bootstrap.
ALTER TABLE users
    ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN phone_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- OAuth is already cryptographically verified by the provider. Preserve that
-- fact for existing accounts when this forward migration is applied.
UPDATE users AS u
SET email_verified = TRUE
FROM user_oauth_identities AS i
WHERE i.user_id = u.id
  AND i.email_verified = TRUE;
