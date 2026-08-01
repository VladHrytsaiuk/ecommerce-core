-- Перед поверненням NOT NULL ми маємо чимось заповнити NULL, якщо вони є. 
-- Але це деструктивна операція, тому просто повертаємо як було.
UPDATE "user" SET phone = '' WHERE phone IS NULL;
ALTER TABLE "user" ALTER COLUMN phone SET NOT NULL;
