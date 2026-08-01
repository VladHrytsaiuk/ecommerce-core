# Інфраструктура та Сервіси (Infrastructure)

У директоріях `internal/platform`, `internal/integration` та `internal/http/middleware` знаходиться інфраструктурний код проекту. Цей код безпосередньо не містить бізнес-логіки продукту, але забезпечує роботу середовища та зв'язок зі світом.

---

## 1. База Даних (PostgreSQL & GORM)

Підключення до бази даних реалізується в `internal/platform/db/postgres.go` через функцію `db.Connect(dsn string)`.
Рядок підключення береться з environment variable `DB_URL` (з файлу `.env` або системних змінних). Наприклад:
`DB_URL=postgres://user:password@localhost:5432/aquawheel-store?sslmode=disable`

### Використання GORM Scopes
В проєкті (див. `internal/shared/pagination/pagination.go`) підготовлено структуру `pagination.Params`. Репозиторії самостійно формують запити з її врахуванням. Наприклад:
```go
func (r *CategoryRepository) GetCategories(ctx context.Context, p *pagination.Params) ([]domain.Category, error) {
    db := r.db.WithContext(ctx).Limit(p.Limit).Offset(p.GetOffset()) // Тут застосовується ліміт і офсет
    // ...
}
```
Це дозволяє консистентно віддавати на фронтенд стуркутуру з `metadata` (кількість сторінок тощо).

---

## 2. Відправка Email (`internal/integration/email`)

Для надсилання листів розроблено фасад `email.Provider` (в `internal/platform/email`). Система ініціалізується у `router.go` та дозволяє гнучко переключати провайдерів за допомогою конфігурації у файлі `.env`:

1. **SendGridProvider:** Активується викликом `email.NewSendGridProvider(cfg)`, якщо в `.env` вказаний `SENDGRID_API_KEY`. Використовує клієнт `github.com/sendgrid/sendgrid-go` для надсилки листів (оптимально для Production). Відправник налаштовується змінною `EMAIL_FROM`.
2. **SMTPProvider:** Активується через `email.NewSMTPProvider(cfg)`, якщо вказані `SMTP_HOST` та `SMTP_USER`. Це резервний варіант.
3. **ZapProvider (Console Logger):** Створюється викликом `email.NewZapProvider()`, якщо жоден ключ не задано. Email-повідомлення успішно "рендеряться" та логуються в консоль через логер `zap` (використовується в dev-mode для швидких перевірок токенів підтвердження).

---

## 3. WebSocket Hub (`internal/platform/notification`)

Реалізований для миттєвого сповіщення клієнтів (знаходиться в `internal/platform/notification/ws_hub.go`). Наразі використовується для **Митієвого підтвердження реєстрації**.

**Як це працює:**
- `router.go` створює єдиний екземпляр: `wsHub := notification.NewHub(logger.Log)`.
- Цей хаб передається у `AuthHandler` (маршрут `/api/auth/ws`). Коли клієнт робить туди GET запит після логіну (з WebSocket клієнта фронтенду), конекшн апгрейдиться бібліотекою `github.com/gorilla/websocket`.
- `Hub` зберігає карту підключень `map[uuid.UUID][]*Client` (відповідність `user_id` до масиву з'єднань, бо може бути кілька вкладок).
- Користувач підтверджує email посиланням `/api/auth/verify`. Після оновлення БД визивається метод хабу:
  ```go
  hub.NotifyUser(userID, map[string]string{"type": "email_verified", "status": "success"})
  ```
- Хаб пушить це JSON-повідомлення у відкритий сокет клієнта. Front-end миттєво реагує та пускає користувача.

---

## 4. Конфігурація (`internal/platform/config`)

Для завантаження змінних з `.env` чи системних середовищ використовується пакет `internal/platform/config/config.go`, який повертає структуру `Config` з полями: `Port`, `DBURL`, `TokenSymmetricKey` (для JWT), `FrontendURL`, `SendGridAPIKey`, `NovaPoshtaAPIKey` тощо. 
Основні бібліотеки — стандартні засоби Go (`os.Getenv`), доповнені Godotenv для локального середовища.

---

## 5. Middleware (`internal/http/middleware`)

Всі Middleware (в `internal/http/middleware/`) згруповані для контролю потоку HTTP-запитів:

- `AuthMiddleware(tokenMaker)`: Обов'язкова перевірка Authorization header. Перевіряє JWT токен (тип `Bearer`), підписаний `TokenSymmetricKey`. При успіху зберігає у контекст `c.Set("user_id", payload.UserID)` та `c.Set("role", payload.Role)`.
- `OptionalAuthMiddleware()`: Парсить токен, якщо він є в хедері (для збереження `user_id`), але якщо його немає — викликає `c.Next()` без помилки. Потрібно для `/api/uk/cart`, куди можуть ходити як зареєстровані, так і гості.
- `SessionMiddleware(cookieSecure)`: Працює в парі з OptionalAuth. Перевіряє чи є в запиті кука `X-Session-ID`. Якщо немає, генерує новий UUID (`uuid.New()`) і ставить її через `c.SetCookie`. Прапорець `cookieSecure` залежить від `FrontendURL` (HTTPS чи ні).
- `LocaleMiddleware()`: Обробляє роути типу `/:lang/...`. Витягує `c.Param("lang")` (зазвичай `uk` чи `en`) та кладе в контекст `c.Set("lang", lang)`.
- `AdminMiddleware()`: Зчитує `c.MustGet("role")`. Якщо роль не дорівнює `"ADMIN"`, викидає `c.AbortWithStatusJSON` статус 403. Вимагає, щоб `AuthMiddleware` був викликаний перед ним.
