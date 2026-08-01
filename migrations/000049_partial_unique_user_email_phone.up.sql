-- Робимо унікальність email/phone частковою: ігноруємо soft-deleted акаунти.
--
-- Раніше user_email_key / user_phone_key (міграція 000003) були звичайними UNIQUE-
-- констрейнтами, що враховували й soft-deleted рядки (deleted_at IS NOT NULL). Через
-- це видалений акаунт назавжди лочив свій email і телефон — повторна реєстрація або
-- верифікація телефону з тим самим значенням падала з unique_violation (23505).
--
-- Часткові унікальні індекси, обмежені активними рядками (deleted_at IS NULL),
-- виправляють це повністю на рівні БД. Код застосунку не змінюється: FindByEmail/
-- FindByPhone і так працюють зі скоупом deleted_at IS NULL, тож soft-deleted юзери
-- для застосунку вже невидимі.
--
-- ВАЖЛИВО: імена індексів збігаються зі старими констрейнтами (user_email_key,
-- user_phone_key). handleUserUniqueErr мапить помилку 23505 за pgErr.ConstraintName,
-- а для часткового унікального індексу Postgres повертає саме імʼя індексу — тож
-- доменні помилки ErrEmailAlreadyExists / ErrPhoneAlreadyExists лишаються робочими
-- для активних дублікатів.

ALTER TABLE "user" DROP CONSTRAINT IF EXISTS user_email_key;
ALTER TABLE "user" DROP CONSTRAINT IF EXISTS user_phone_key;

CREATE UNIQUE INDEX user_email_key ON "user" (email) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX user_phone_key ON "user" (phone) WHERE deleted_at IS NULL;
