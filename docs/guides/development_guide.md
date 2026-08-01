# Посібник розробника: Тестування, Міграції та Swagger

Цей документ описує стандарти та інструменти, які використовуються під час розробки нових фіч та підтримки поточного коду проєкту `AquaWheel Store`.

---

## 1. Міграції Бази Даних (`migrations/`)

Проєкт використовує пакет `github.com/golang-migrate/migrate/v4` для управління схемою бази даних PostgreSQL (відмова від GORM AutoMigrate заради стабільності виробничого середовища та контролю над індексами).

- **Розташування:** Всі міграції знаходяться у теці `migrations/`.
- **Формат:** Файли завжди йдуть парами: `<sequence_number>_<name>.up.sql` та `<sequence_number>_<name>.down.sql` (наприклад, `000001_create_users_table.up.sql`).
- **Як запускати локально (CLI):** 
  Якщо у вас встановлено утиліту `migrate` (через `brew install golang-migrate` або іншим способом), ви можете застосувати міграції командою:
  ```bash
  migrate -path migrations -database "postgres://user:password@localhost:5432/aquawheel-store?sslmode=disable" up
  ```
  *(Вкажіть ваш `DB_URL` у параметрі `database`).*
- **Створення нової міграції:** 
  ```bash
  migrate create -ext sql -dir migrations -seq <nazva_migratsii>
  ```
- **Скрипт `cmd/migrate`:** В застосунку є спеціальна точка входу пакету `main` для міграцій — файл `cmd/migrate/main.go`. Його можна запустити або зібрати (скомпілювати) для автоматичного виконання міграцій перед запуском основного API сервера: `go run cmd/migrate/main.go`.

---

## 2. Тестування (`*_test.go`)

Тести поділяються на дві основні категорії: **Unit-тести** (для service та delivery/http шарів) та **Інтеграційні тести** (для repository шару).

### 2.1. Unit-тести (Mockery + Testify)
У цих тестах (наприклад, `internal/cart/delivery/http/cart_handler_test.go`) ми ізолюємо логіку за допомогою згенерованих моків.
- **Testify:** Використовує бібліотеки `github.com/stretchr/testify/assert` та `require` для перевірок. Приклад: `assert.NoError(t, err)`.
- **Mockery:** Автоматично генерує імплементації інтерфейсів на основі коментарів `//go:generate mockery`. Наприклад, для `UserService` ми використовуємо інжектований `userRepo *mocks.UserRepository` і налаштовуємо його поведінку:
  ```go
  mockRepo.On("CreateUser", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
  ```

### 2.2. Інтеграційні тести (Testcontainers)
Оскільки `*Repository` активно взаємодіє з базою даних, мокати її — погана практика. Натомість використовуються **Testcontainers** (пакет `github.com/testcontainers/testcontainers-go`).
- **Як це працює (на прикладі `user_repository_test.go`):** 
  1. При старті тесту (у `TestMain` або `setupTestDB`) піднімається Docker-контейнер з образом `postgres:15-alpine`.
  2. Застосовуються SQL-міграції з папки `migrations/` безпосередньо в контейнер.
  3. Ініціалізується GORM клієнт, підключений до відкритого порту контейнера.
  4. Виконуються ваші тести (`userRepo.CreateUser(...)`).
  5. У блоці `defer container.Terminate(ctx)` контейнер безпечно видаляється після тестів.
- Це гарантує, що всі SQL/GORM запити є сумісними з реальною версією PostgreSQL на Production.

> [!TIP]
> Для запуску всіх тестів необхідно, щоб Docker Desktop (або OrbStack/Colima) був запущений локально. Команда запуску: `go test ./... -v`

---

## 3. Swagger Документація

Документація API автоматично генерується інструментом **swaggo/swag** (репозиторій `github.com/swaggo/swag`).

- **Анотації:** Перед кожним хендлером (`Handler`) знаходяться коментарі спеціального формату.
  ```go
  // @Summary Отримати профіль
  // @Tags Управління Користувачами
  // @Security BearerAuth
  // @Produce json
  // @Success 200 {object} dto.UserResponse "Дані профілю"
  // @Failure 401 {object} errors.ErrorResponse "Не авторизовано"
  // @Router /api/users/profile [get]
  ```
- **Генерація:** Щоразу, коли ви додаєте новий хендлер, змінюєте DTO або міняєте маршрутизацію (файл `router.go`), необхідно оновити Swagger маніфести. З кореня проєкту виконайте:
  ```bash
  swag init -g cmd/api/main.go --parseDependency --parseInternal
  ```
- **Результат:** Оновляться файли: `docs/swagger.json`, `docs/swagger.yaml` та `docs/docs.go`. 
  У браузері документація доступна за допомогою `gin-swagger` за адресою: `http://localhost:8080/swagger/index.html`.
