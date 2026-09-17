-- One-time codes for signing in with an email address, without a password.
--
-- Requesting a code and entering it is both registration and sign-in: the
-- address that receives the code is the account. Neither the address nor the
-- code is stored. destination_hash and code_hash are HMACs under keys derived
-- from JWT_SECRET, so a copy of this table names nobody and, holding only six
-- digits' worth of entropy per code, cannot be brute-forced without the key.
--
-- Rows are kept for a day after they are issued, closed or not, because the
-- per-address limits on how many codes may be sent are counted from them.
CREATE TABLE sign_in_codes (
    id UUID PRIMARY KEY,
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('email')),
    destination_hash BYTEA NOT NULL,
    code_hash BYTEA NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    -- closed_at is set when the code is used, replaced by a newer code, or out
    -- of attempts. A closed code is never accepted.
    closed_at TIMESTAMPTZ,
    CONSTRAINT sign_in_codes_destination_hash_is_sha256 CHECK (octet_length(destination_hash) = 32),
    CONSTRAINT sign_in_codes_code_hash_is_sha256 CHECK (octet_length(code_hash) = 32),
    CONSTRAINT sign_in_codes_expire_after_creation CHECK (expires_at > created_at)
);

-- Issuing and verifying look up an address's codes, newest first.
CREATE INDEX sign_in_codes_destination_idx ON sign_in_codes (channel, destination_hash, created_at DESC);
-- The purge removes rows older than the rate-limit window, oldest first.
CREATE INDEX sign_in_codes_created_idx ON sign_in_codes (created_at);
