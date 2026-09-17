-- Codes now also verify an address after registration and reset a password,
-- so the table is renamed for what it holds and each code records its purpose.
-- A code is accepted only for the purpose it was issued for. The sending limits
-- stay per address across every purpose: each one is a message to the same
-- inbox.
ALTER TABLE sign_in_codes RENAME TO one_time_codes;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_pkey TO one_time_codes_pkey;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_channel_check TO one_time_codes_channel_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_attempts_check TO one_time_codes_attempts_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_max_attempts_check TO one_time_codes_max_attempts_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_destination_hash_is_sha256 TO one_time_codes_destination_hash_is_sha256;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_code_hash_is_sha256 TO one_time_codes_code_hash_is_sha256;
ALTER TABLE one_time_codes RENAME CONSTRAINT sign_in_codes_expire_after_creation TO one_time_codes_expire_after_creation;
ALTER INDEX sign_in_codes_destination_idx RENAME TO one_time_codes_destination_idx;
ALTER INDEX sign_in_codes_created_idx RENAME TO one_time_codes_created_idx;

-- Every code issued before this migration was a sign-in code.
ALTER TABLE one_time_codes
    ADD COLUMN purpose VARCHAR(32) NOT NULL DEFAULT 'sign_in'
        CONSTRAINT one_time_codes_purpose_check CHECK (purpose IN ('sign_in', 'verify_email', 'reset_password'));
ALTER TABLE one_time_codes ALTER COLUMN purpose DROP DEFAULT;

-- Verifying a code looks up the address's newest open code for one purpose.
CREATE INDEX one_time_codes_purpose_idx ON one_time_codes (channel, destination_hash, purpose, created_at DESC);
