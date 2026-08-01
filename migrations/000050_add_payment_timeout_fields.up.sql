ALTER TABLE "order" ADD COLUMN IF NOT EXISTS payment_reminder_sent_at TIMESTAMPTZ;
