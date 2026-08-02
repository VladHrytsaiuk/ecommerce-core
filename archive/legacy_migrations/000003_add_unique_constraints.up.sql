ALTER TABLE "user"
ADD CONSTRAINT user_email_key UNIQUE (email),
ADD CONSTRAINT user_phone_key UNIQUE (phone);
