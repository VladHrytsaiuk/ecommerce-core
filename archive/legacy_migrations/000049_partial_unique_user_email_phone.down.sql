-- Відкат: повертаємо звичайні (не часткові) UNIQUE-констрейнти на email/phone.
--
-- УВАГА: якщо за час дії часткових індексів накопичились дублікати email або phone
-- між активними та soft-deleted рядками, цей відкат впаде на unique_violation — це
-- очікувано для зворотної міграції. Такі дублікати треба прибрати вручну перед down.

DROP INDEX IF EXISTS user_email_key;
DROP INDEX IF EXISTS user_phone_key;

ALTER TABLE "user"
    ADD CONSTRAINT user_email_key UNIQUE (email),
    ADD CONSTRAINT user_phone_key UNIQUE (phone);
