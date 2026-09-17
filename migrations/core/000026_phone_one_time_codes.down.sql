-- Phone codes cannot be represented once the channel is gone.
DELETE FROM one_time_codes WHERE channel = 'phone';
DROP INDEX IF EXISTS one_time_codes_channel_created_idx;
ALTER TABLE one_time_codes
    DROP CONSTRAINT one_time_codes_channel_check,
    ADD CONSTRAINT one_time_codes_channel_check CHECK (channel IN ('email'));
