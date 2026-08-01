# 🛠️ Setup Guide: Налаштування локального середовища

Цей розділ описує, як запустити і налаштувати AquaWheel Store Backend у локальному середовищі.

## Що вже підключено та налаштовано

- **Go 1.25+** — основна мова розробки, встановлена як runtime.
- **Gin** — HTTP‑framework для REST‑API.
- **PostgreSQL** — основна база даних, наразі розгорнута через **Supabase**.
- **GORM** — ORM для роботи з PostgreSQL.
- **Uber-go/Zap** — швидке структуроване логування.
- **Swagger** — генерація та документація API.
- **Docker** та **docker-compose** — контейнеризація та локальне оточення (API, БД, фонові процеси).
- **Testify** та **Mockery** — юніт‑тести та mock‑об’єкти для інтерфейсів.
- **Testcontainers** — інтеграційні тести з реальним PostgreSQL‑контейнером.

***

## Підготовка оточення

### 1. Встановлення залежностей

1. Встанови **Go 1.25 або новіше**:
    - офіційний інсталятор: https://go.dev/dl/
2. Встанови **Docker** та **docker-compose**:
    - Docker: https://www.docker.com/get-started
3. Переконайся, що в PATH є:
    - `go`
    - `docker`
    - `docker-compose`

### 2. Клонування репозиторію

```bash
git clone https://gitlab.com/oleksiibehun-group/aquawheel-store.git
cd aquawheel-store
```

### 3. Налаштування `.env`

Скопіюй приклад:

```bash
cp .env.example .env
```

Відкрий `.env` у редакторі та заповни мінімальні змінні:

```bash
PORT=8080
DB_URL=postgres://user:password@host:port/dbname
```

- `DB_URL` — адреса до PostgreSQL (Supabase або власний сервер).  
  Для Supabase зазвичай виглядає як:
  ```bash
  DB_URL=postgres://postgres:your_password@db.your_project.supabase.co:6543/postgres
  ```

***

## Запуск через Docker (рекомендований спосіб)

Якщо ти використовуєш Docker compose‑файл:

1. Переконайся, що Docker‑сокет доступний.
2. Запусти сервіси:

```bash
docker-compose up --build
```

Після запуску:

- API буде доступне за адресою:
  ```bash
  http://localhost:8080
  ```
- Swagger‑документація:
  ```bash
  http://localhost:8080/swagger/index.html
  ```

***

## Запуск без Docker (локальний Go)

Якщо ти хочеш запускати без Docker:

1. Впевнись, що БД доступна за `DB_URL` (PostgreSQL‑сервер або Supabase).
2. У корені проєкту виконай:

```bash
go run cmd/api/main.go
```

Або збірка та запуск:

```bash
go build -o aquawheel cmd/api/main.go
./aquawheel
```

API також запуститься на `http://localhost:8080` (порт можна змінити через `PORT` у `.env`).

***

## Ініціалізація бази даних

Проєкт використовує:

- **SQL-міграції** у папці `migrations/`.
- Інструмент для запуску міграцій у `cmd/migrate/`.

1. Переконайся, що БД доступна.
2. Запусти міграції (через Docker‑контейнер або локально):

```bash
docker-compose run --rm migrate
```

або, якщо використовуєш Go без Docker:

```bash
go run cmd/migrate/main.go
```

Після цього до БД застосуються таблиці та необхідна структура схеми.

***

## Запуск тестів

Проєкт включає:

- **unit-тести** (через Testify + Mockery);
- **інтеграційні тести** з PostgreSQL‑контейнером (через Testcontainers).

1. Запуск усіх тестів:

```bash
go test ./... -v
```

2. Testcontainers самостійно підніме тимчасовий PostgreSQL‑контейнер, виконає тести й видалить його.

***

## Коротко: один путь запуску (рекомендований)

1. Встановити **Go**, **Docker**, **docker-compose**.
2. Клонувати репу.
3. Створити та налаштувати `.env`.
4. Запустити через Docker Compose:

```bash
docker-compose up --build
```

5. Відкрити в браузері:
    - API: `http://localhost:8080`
    - Swagger: `http://localhost:8080/swagger/index.html`
6. (Опціонально) запустити міграції:

```bash
docker-compose run --rm migrate
```
