ALTER TABLE "user" ALTER COLUMN phone DROP NOT NULL;
UPDATE "user" SET phone = NULL WHERE phone = '';
