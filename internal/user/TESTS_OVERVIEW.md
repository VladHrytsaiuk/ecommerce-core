# 🛡️ Повний огляд тестів модуля User (AquaWheel Store)

Цей документ містить опис усіх тестових сценаріїв, які забезпечують надійність системи реєстрації, авторизації та управління користувачами.

---

## 1. Рівень безпеки та платформенних утиліт (`internal/platform/security/`)
Тут перевіряються "цеглинки", на яких тримається вся безпека: паролі та токени.

### Хешування паролів (`password/hash_test.go`)
1.  **HashPassword**: Перевірка успішного хешування через bcrypt.
2.  **VerifyPassword**: Перевірка відповідності пароля хешу.
3.  **WrongPassword**: Гарантія того, що невірний пароль ніколи не пройде.
4.  **EmptyPassword**: Обробка помилок при порожньому паролі.
5.  **VeryLongPassword**: Перевірка обмежень bcrypt (макс. 72 символи).

### JWT Токени (`token/jwt_test.go`)
1.  **GenerateAccessToken**: Створення токена з правильними Claims (UserID, RoleID).
2.  **ParseToken**: Успішна дешифрація токена.
3.  **ExpiredToken**: Перевірка, що прострочені токени відхиляються.
4.  **InvalidSignature**: Захист від підробки підпису токена.
5.  **WrongIssuer**: Перевірка безпеки джерела випуску токена.
6.  **RefreshTokens**: Окремі тести для генерації довших за часом життя Refresh-токенів.

---

## 2. Доменна логіка (`internal/user/domain/`)
Перевірка базових сутностей та помилок.

### Domain Entity Tests (`user_test.go`)
1.  **User_DefaultValues**: Чи правильно заповнюються поля за замовчуванням.
2.  **DomainErrors**: Чи всі доменні помилки (`ErrUserNotFound` тощо) існують та мають унікальні повідомлення.
3.  **TableName**: Перевірка, що GORM мапить таблицю `"user"` саме в лапках (захист від SQL-слів).

---

## 3. Сервісний шар (`internal/user/service/`)
Тут живе вся бізнес-логіка. Це Unit-тести з використанням Моків (Mocks).

### AuthService (`auth_service_test.go`) — найважливіші тести:
1.  **Register_HappyPath**: Повний шлях успішної реєстрації.
2.  **Register_DuplicateEmail**: Заборона реєстрації з існуючим email.
3.  **Register_ShortPassword**: Валідація довжини пароля.
4.  **Login_Success**: Успішний вхід.
5.  **Login_WrongPassword**: Відмова у доступі при помилці в паролі.
6.  **Login_UserBlocked**: Заборона входу для заблокованих юзерів.
7.  **VerifyEmail_Success**: Успішна активація акаунта через код.
8.  **VerifyEmail_InvalidCode**: Відхилення невірного коду.
9.  **ForgotPassword / ResetPassword**: Повний цикл відновлення доступу (6+ тестів на різні стадії).
10. **Refresh_Tokens**: Обмін Refresh-токена на новий Access-пакет.
11. **Logout**: Видалення сесії з бази.

### UserService (`user_service_test.go`):
1.  **GetMe_Success**: Отримання власного профілю.
2.  **UpdateMe_ProtectedFields**: Перевірка, що юзер не може сам собі змінити `RoleID` або статус `IsBlocked`.
3.  **Address_Management**: Створення адрес, вибір "дефолтної", видалення (8+ тестів).
4.  **DeleteMe**: Логіка повного видалення акаунта разом із сесіями.

---

## 4. Рівень HTTP (Delivery) (`internal/user/delivery/http/`)
Перевірка того, як API спілкується із зовнішнім світом.

### AuthHandler / UserHandler (`auth_handler_test.go`):
1.  **JSON_Binding**: Тести на неправильний JSON (400 Bad Request).
2.  **Status_Codes**: Чи повертає API 409 при конфлікті, 401 при невірному логіні, 201 при створенні.
3.  **Response_Structure**: Чи немає в JSON зайвих полів (наприклад, PASSWORD_HASH не повинен бути у відповіді).
4.  **Validation_Errors**: Перевірка обов'язкових полів у запитах.

---

## 5. Інтеграційні тести (Repository) (`internal/user/repository/postgres/`)
Це тести з реальною базою даних у Docker (Testcontainers).

1.  **UserRepository_Integration**: Чи працюють Unique constraints у БД, чи працює Soft Delete.
2.  **SessionRepository_Integration**: Чи видаляються прострочені сесії з Postgres автоматично.
3.  **AddressRepository_Integration**: Чи скидається статус `is_default` у старих адрес, коли додається нова дефолтна.
4.  **VerifyCode_Integration**: Робота кодів підтвердження (створення, використання, закінчення терміну).

---

## 🚀 Чому це важливо?
Загалом маємо близько **150+ сценаріїв**. 
- **100% перевірена безпека**: Жоден юзер не зайде без пароля або з чужим токеном.
- **Стабільність БД**: Ми впевнені, що міграції та SQL-запити до Postgres працюють коректно.
- **Швидкість**: Unit-тести (120+) проходять за 1 сек, Інтеграційні (15+) за 5-7 сек.
