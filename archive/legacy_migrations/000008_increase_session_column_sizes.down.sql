-- Повернення до 255 символів може призвести до помилки, якщо дані вже існують.
-- У такому випадку Postgres не дозволить зменшити тип без обрізання.
ALTER TABLE "session" ALTER COLUMN refresh_token TYPE VARCHAR(255);
ALTER TABLE "session" ALTER COLUMN user_agent TYPE VARCHAR(255);
