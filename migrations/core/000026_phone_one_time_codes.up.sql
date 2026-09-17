-- One-time codes are now also texted to phone numbers, for phone sign-in.
ALTER TABLE one_time_codes
    DROP CONSTRAINT one_time_codes_channel_check,
    ADD CONSTRAINT one_time_codes_channel_check CHECK (channel IN ('email', 'phone'));

-- The store-wide hourly cap on texts counts a channel's codes in the last hour.
CREATE INDEX one_time_codes_channel_created_idx ON one_time_codes (channel, created_at);
