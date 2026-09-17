-- Codes for any other purpose cannot be represented once the column is gone.
DELETE FROM one_time_codes WHERE purpose <> 'sign_in';
DROP INDEX IF EXISTS one_time_codes_purpose_idx;
ALTER TABLE one_time_codes DROP COLUMN purpose;
ALTER INDEX one_time_codes_created_idx RENAME TO sign_in_codes_created_idx;
ALTER INDEX one_time_codes_destination_idx RENAME TO sign_in_codes_destination_idx;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_expire_after_creation TO sign_in_codes_expire_after_creation;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_code_hash_is_sha256 TO sign_in_codes_code_hash_is_sha256;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_destination_hash_is_sha256 TO sign_in_codes_destination_hash_is_sha256;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_max_attempts_check TO sign_in_codes_max_attempts_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_attempts_check TO sign_in_codes_attempts_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_channel_check TO sign_in_codes_channel_check;
ALTER TABLE one_time_codes RENAME CONSTRAINT one_time_codes_pkey TO sign_in_codes_pkey;
ALTER TABLE one_time_codes RENAME TO sign_in_codes;
