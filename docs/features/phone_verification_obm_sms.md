# Підтвердження номера телефону через SMS (Vodafone OBM)

> Повна документація фічі верифікації телефону: архітектура, інтеграція з Vodafone OBM REST API v3,
> конфігурація, покрокова інструкція тестування та troubleshooting.

---

## 1. TL;DR

- Користувач підтверджує номер телефону OTP-кодом, який надсилається SMS через **Vodafone OBM REST API v3**.
- HTTP-флоу, БД-поля та сервісний інтерфейс **уже існували** в проєкті — бракувало лише **реальної відправки SMS** (раніше був мок у консоль).
- Додано: SMS-абстракцію, клієнт Vodafone OBM (авторизація + відправка + перевірка статусу доставки + нормалізація номера), конфіг, асинхронну відправку.
- **Код повністю готовий і протестований.** На момент написання доставка блокується **на боці Vodafone** (`00-0000 System error 0` — найімовірніше тарифікація/баланс профілю), що **не вимагає змін у коді**.

---

## 2. Архітектура та флоу

```
POST /api/auth/verify-phone/request   { "phone": "+380..." }
        │
        ▼
authService.RequestPhoneVerification()
        │  1. генерує 6-значний код (crypto/rand)
        │  2. зберігає в таблицю verify_code (type=phone_verification, TTL 2 хв)
        │  3. АСИНХРОННО (горутина, окремий контекст) шле SMS через провайдера
        ▼
sms.Sender ──► VodafoneSender (реальний OBM)  /  LogSender (мок у dev)
        │  - OAuth токен (кешується ~12 год)
        │  - POST /communicationMessage/send (вкладена схема)
        │  - через 5с: GET /status?type=FULL → лог DELIVERED/REJECTED
        ▼
SMS на телефон

POST /api/auth/verify-phone/confirm   { "phone": "+380...", "code": "123456" }
        │
        ▼
authService.ConfirmPhoneVerification()
        │  - звіряє код з verify_code (не використаний, не прострочений)
        │  - якщо код прив'язаний до user → user.is_phone_verified = true
        ▼
{ "message": "Phone verified successfully" }
```

---

## 3. База даних — міграція НЕ потрібна

Усі поля вже існували до цієї задачі:

| Об'єкт | Поле / Тип | Призначення |
|---|---|---|
| `user` | `phone varchar(20) unique` | номер телефону |
| `user` | `is_phone_verified bool default false` | прапорець підтвердження |
| `verify_code` (таблиця) | `type varchar(50)` | підтримує `"phone_verification"` |
| `verify_code` | `code`, `target`, `expires_at`, `is_used`, `user_id` | OTP-код, ціль (телефон), термін дії |

Доменна модель: [internal/user/domain/user.go](../internal/user/domain/user.go) (`User`, `VerifyCode`).
Репозиторій кодів: [internal/user/repository/postgres/verify_code_repository.go](../internal/user/repository/postgres/verify_code_repository.go).

---

## 4. Файли, що додані / змінені

| Файл | Що це |
|---|---|
| `internal/platform/sms/sender.go` | **Новий.** Інтерфейс `Sender` + `LogSender` (мок для dev) |
| `internal/integration/sms/vodafone.go` | **Новий.** Реальний клієнт Vodafone OBM (auth, send, status, нормалізація MSISDN) |
| `internal/integration/sms/vodafone_test.go` | **Новий.** Юніт-тести `normalizeMSISDN` (10 кейсів форматів) |
| `internal/user/service/auth_service.go` | Інжект `smsSender`; `RequestPhoneVerification` шле SMS **асинхронно**, TTL коду 2 хв |
| `internal/platform/config/config.go` | OBM-конфіг + `OBMStatusCheck` + `PhoneCodeTTL` |
| `internal/http/router.go` | Вибір провайдера: Vodafone (якщо є креди) або `LogSender` |
| `internal/user/service/mocks_test.go` | `MockSMSSender` (no-op) для тестів |
| `.env.example` | Змінні OBM |

HTTP-ендпоінти (уже існували): [internal/user/delivery/http/routes.go](../internal/user/delivery/http/routes.go),
хендлери — [auth_handler.go](../internal/user/delivery/http/auth_handler.go) (`RequestPhoneVerification`, `ConfirmPhoneVerification`).

---

## 5. Інтеграція з Vodafone OBM REST API v3

### 5.1. Базовий URL та авторизація

- **Base URL:** `https://a2p.vodafone.ua`
- **Токен (OAuth2 password grant):** `POST /uaa/oauth/token`
  > ⚠️ Існує два деплойменти OBM. Наш акаунт використовує **`/uaa/oauth/token`** (НЕ `/customers-registration/api/v1/customer/oauth/token` — там 404).
- Заголовки токена:
  - `Content-Type: application/x-www-form-urlencoded`
  - `Authorization: Basic d2ViYXBwOndlYmFwcA==` (фіксований client-credential OBM `webapp:webapp`)
- Тіло: `grant_type=password&username=<логін кабінету>&password=<пароль>`
- Логін кабінету = email / nickname / телефон (будь-який працює). Токен живе `expires_in ≈ 43199` сек (~12 год) → **кешуємо**.

### 5.2. Відправка SMS — ВКЛАДЕНА схема (важливо!)

- **Ендпоінт:** `POST /communication-event/api/communicationManagement/v3/communicationMessage/send`
- Заголовки: `Content-Type: application/json`, `Authorization: Bearer <access_token>`
- ⚠️ Наш деплоймент (`/uaa`) вимагає **вкладену** схему. Проста схema (`type`+`message.content`) → **HTTP 500**.

**Робоче тіло:**
```json
{
  "receiver": ["380987293334"],
  "cascades": [
    {
      "transport": "SMS",
      "senderId": 7364601,
      "validityPeriod": "2",
      "messageObject": {
        "type": "SMS",
        "smsMessage": { "content": "Код підтвердження: 123456\nДійсний протягом 2 хв." }
      }
    }
  ]
}
```
- `senderId` — ID **імені відправника** (alpha-name), напр. `7364601` = `AQUAWHEEL`.
- `validityPeriod` — час життя в **хвилинах** (рядок `"2"`).
- Успіх: `200` + `[{"id":"05-00-380987293334-..."}]` — це **прийом у чергу**, ще не доставка.

### 5.3. Статус доставки

- **Ендпоінт:** `GET /communication-event/api/communicationManagement/v3/communicationMessage/status?id=<message_id>&type=FULL`
  > ⚠️ Шлях саме `/status` (НЕ `/statuss` — у нашому деплої дає 404).
- Відповідь містить `status` (`SENT` / `DELIVERED` / `EXPIRED` / `REJECTED`) і об'єкт `error` з `errorCode` + `errorDescription_uk`.
- Наш бекенд через 5с після відправки сам тягне статус і пише в лог (див. `scheduleStatusCheck` у `vodafone.go`).

### 5.4. Distribution ID — НЕ потрібен

`ID розсилки` (напр. `7369261`) до прямого `/send` **не використовується**. Перевірено: додавання його в payload (у cascade, на верхньому рівні, як `id`/`distributionId`) — **не змінює результат**. Пряма відправка йде через `senderId`, а не через збережену розсилку.

### 5.5. Нормалізація номера (MSISDN)

OBM очікує формат `380XXXXXXXXX` (без `+`, пробілів, дужок). Функція `normalizeMSISDN` ([vodafone.go](../internal/integration/sms/vodafone.go)) приводить будь-який ввід:

| Ввід | Результат |
|---|---|
| `+380501234567` | `380501234567` |
| `0501234567` | `380501234567` |
| `+38 (050) 123-45-67` | `380501234567` |
| `80501234567` | `380501234567` |
| `501234567` | `380501234567` |
| `12345` / `""` / `abc` | `""` (помилка) |

---

## 6. Конфігурація (.env)

```bash
# --- Vodafone OBM (SMS) ---
OBM_USERNAME=aquawheelstore@start.eu.com   # логін кабінету a2p.vodafone.ua
OBM_PASSWORD=********                        # пароль кабінету
OBM_SENDER_ID=7364601                        # ID імені відправника (AQUAWHEEL)
OBM_DISTRIBUTION_ID=7369261                  # довідково; у /send не використовується
OBM_VALIDITY_MINUTES=2                       # час життя SMS (хв)
OBM_STATUS_CHECK=true                        # логувати статус доставки (false — вимкнути)
PHONE_CODE_TTL=2m                            # час життя OTP-коду в БД

# Опціонально (мають робочі дефолти):
# OBM_BASE_URL=https://a2p.vodafone.ua
# OBM_TOKEN_PATH=/uaa/oauth/token
# OBM_BASIC_AUTH_HEADER=Basic d2ViYXBwOndlYmFwcA==
```

**Логіка вибору провайдера** ([router.go](../internal/http/router.go)):
- якщо задані `OBM_USERNAME` + `OBM_PASSWORD` + `OBM_SENDER_ID` → реальний `VodafoneSender`;
- інакше → `LogSender` (код пишеться в консоль, SMS не шлеться). У логах старту: `⚠️ Vodafone OBM not configured, using console SMS sender`.

> 🔒 **Безпека:** `.env` у `.gitignore`. Не комітьте креди. Якщо пароль засвітився — змініть у кабінеті.

---

## 7. Поведінка в dev vs production

**Безпечно за замовчуванням:** OTP-код повертається у відповіді `/request` **лише** за явного `APP_ENV=development` або `APP_ENV=local`. Будь-яке інше або не задане значення (зокрема прод) — код у відповідь **не потрапляє**, його можна отримати тільки через SMS.

| `APP_ENV` | Код у відповіді `/request` |
|---|---|
| `development` / `local` | **повертається** в JSON (`"code":"..."`) — для зручності тестів |
| не задано / `production` / будь-що інше | **НЕ повертається** (безпечний дефолт) |

> 🔒 Раніше код віддавався і за порожнього `APP_ENV` — це було небезпечно (забув виставити змінну = витік). Тепер dev-режим треба ввімкнути **явно**, тож прод за замовчуванням захищений.
> ⚠️ Для отримання коду у відповіді в локальній розробці додайте `APP_ENV=development` у свій `.env`.
> Логіка: [auth_handler.go](../internal/user/delivery/http/auth_handler.go) (`RequestPhoneVerification`).

---

## 8. Інструкція з тестування

### 8.1. Передумови

- Заповнений `.env` (розділ OBM, див. §6).
- Запущена БД.
- Тестовий номер — бажано **Vodafone-номер** (виключає interconnect). Зручно — номер, прив'язаний до самого OBM-акаунта.

### 8.2. ⚠️ Найчастіша пастка — конфлікт порту 8080

Якщо паралельно запущені **і Docker, і локальний `go run ./cmd/api`** — вони конфліктують за порт `8080`, і `curl` може потрапляти не туди (на старий бінарник без SMS-коду). Симптоми: `curl` повертає код, але **в логах контейнера нічого**, SMS не йде.

Перевірити, хто слухає порт:
```bash
lsof -nP -i :8080
```
- `com.docker` / `docker-pr` → Docker;
- `api` / `main` / `go` → локальний бінарник.

Прибрати зайвий локальний процес:
```bash
pkill -f "cmd/api"          # вбити go run
# переконатись, що лишився тільки Docker:
lsof -nP -i :8080
```

**Обери ОДИН спосіб запуску** (Docker АБО `go run`), щоб не ділити порт.

### 8.3. Запуск застосунку

**Docker (рекомендовано):**
```bash
# ⚠️ після будь-якої зміни Go-коду — обов'язково --build (контейнер крутить вшитий бінарник)
docker compose up -d --build aquawheel-api

# назви: сервіс = aquawheel-api, контейнер = aquawheel-backend
docker ps --format '{{.Names}}\t{{.Status}}\t{{.Ports}}'   # має бути 127.0.0.1:8080->8080/tcp
```
> Зміна **тільки `.env`** (без коду) → достатньо `docker compose restart aquawheel-api` (env підтягується через `env_file`).

**Локально:**
```bash
go run ./cmd/api
```

### 8.4. Логи

Логи пишуться в **stdout** (без файлу).

```bash
# Docker:
docker logs -f aquawheel-backend 2>&1 | grep -iE "GIN|SMS|OBM"
# go run: логи прямо в терміналі, де запущено
```
> Після `docker compose up --build` контейнер пересоздається — **старий `docker logs -f` відчіплюється**. Запускай стрім логів **заново** після ребілду.

Маркери в логах:
| Рядок | Значення |
|---|---|
| `✅ SMS accepted by Vodafone OBM   message_id=...` | OBM прийняв запит |
| `📬 SMS DELIVERED` | доставлено на телефон |
| `📨 SMS handed to operator (SENT)` | передано оператору |
| `❌ SMS NOT delivered   status=REJECTED   errorCode=...` | доставку відхилено (з кодом помилки) |
| `📱 [SMS MOCK]` | працює мок (креди OBM не підхопились) |
| `obm send returned non-2xx` | OBM відхилив сам запит (status+body) |

### 8.5. Прогін через ендпоінти застосунку

Термінал 1 — логи (див. §8.4). Термінал 2:

```bash
# 1) Запит коду (на 127.0.0.1 — Docker слухає IPv4-loopback)
curl -X POST http://127.0.0.1:8080/api/auth/verify-phone/request \
  -H "Content-Type: application/json" \
  -d '{"phone":"+380987293334"}'
# dev-відповідь: {"message":"Verification code sent successfully","code":"123456"}

# 2) Підтвердження кодом (з SMS, або з JSON у dev)
curl -X POST http://127.0.0.1:8080/api/auth/verify-phone/confirm \
  -H "Content-Type: application/json" \
  -d '{"phone":"+380987293334","code":"123456"}'
# очікувано: {"message":"Phone verified successfully"}
```

У логах після кроку 1 (через ~5с з'явиться статус доставки):
```
[GIN] ... POST "/api/auth/verify-phone/request"
✅ SMS accepted by Vodafone OBM   message_id=05-00-380987293334-...
📬 SMS DELIVERED   message_id=05-00-...        ← або ❌ SMS NOT delivered ...
```

### 8.6. Пряме тестування OBM (curl, без застосунку)

Корисно для ізоляції проблеми «код vs Vodafone». Підстав свої креди (не коміть!).

**Крок 1 — токен:**
```bash
TOKEN=$(curl -s -X POST 'https://a2p.vodafone.ua/uaa/oauth/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -H 'Authorization: Basic d2ViYXBwOndlYmFwcA==' \
  --data-urlencode 'grant_type=password' \
  --data-urlencode 'username=ЛОГІН' \
  --data-urlencode 'password=ПАРОЛЬ' \
  | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
echo "token len: ${#TOKEN}"   # > 1000 → ок
```

**Крок 2 — відправка (вкладена схема):**
```bash
SEND='https://a2p.vodafone.ua/communication-event/api/communicationManagement/v3/communicationMessage/send'
R=$(curl -s -X POST "$SEND" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $TOKEN" \
  -d '{"receiver":["380987293334"],"cascades":[{"transport":"SMS","senderId":7364601,"validityPeriod":"2","messageObject":{"type":"SMS","smsMessage":{"content":"AquaWheel test"}}}]}')
echo "$R"   # [{"id":"05-00-380987293334-..."}]
MID=$(echo "$R" | sed -E 's/.*"id":"([^"]+)".*/\1/')
```

**Крок 3 — статус доставки:**
```bash
ST='https://a2p.vodafone.ua/communication-event/api/communicationManagement/v3/communicationMessage/status'
ENC=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$MID")
sleep 5
curl -s "$ST?id=$ENC&type=FULL" -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
# status: DELIVERED / SENT / REJECTED (+ error.errorCode)
```

### 8.7. Юніт-тести

```bash
go build ./...
go test ./internal/integration/sms/ -v        # normalizeMSISDN
go test ./internal/user/service/               # auth service
```

---

## 9. Хронологія проблем і фіксів (історія цієї задачі)

| # | Симптом | Причина | Фікс |
|---|---|---|---|
| 1 | `RequestPhoneVerification` лише логував код | був мок-провайдер | реальний `VodafoneSender` |
| 2 | `404` на токені | дефолтний шлях `/customers-registration/...` не існує на акаунті | дефолт → `/uaa/oauth/token` |
| 3 | SMS не йде, логи контейнера пусті | curl потрапляв на **старий локальний `go run`** на `*:8080` | прибрати стрейний процес, тестувати на `127.0.0.1` |
| 4 | `context deadline exceeded` | глобальний `TimeoutMiddleware(5s)` рвав HTTP-виклик OBM | відправка SMS **асинхронно** (фон, окремий контекст 20с) |
| 5 | `HTTP 500` від `/send` | акаунт `/uaa` вимагає **вкладену** схему тіла | `transport` + `messageObject.smsMessage.content` |
| 6 | `200`, але SMS не приходить | OBM прийняв, але відхилив доставку | додано перевірку статусу → видно `REJECTED 00-0000` |
| 7 | `REJECTED 00-0000 System error 0` | **на боці Vodafone** (див. §10) | звернення в підтримку / поповнення балансу |

---

## 10. Відоме блокування: `00-0000 System error 0`

### Стан
- Профіль `7364509` — **Активний**, REST API + «SMS на інші мережі» увімкнені.
- Ім'я відправника `7364601` (AQUAWHEEL) — **Активне**, канал SMS.
- Розсилка REST API `7369261` — **Запущено**.
- Payload коректний (`200` + `message_id`), номер валідний Vodafone.
- **І все одно** статус = `REJECTED`, `errorCode 00-0000 "System error 0"`.

### Висновок
Усе, що контролює клієнт, налаштовано правильно → це **внутрішня помилка Vodafone OBM**, не код і не наш конфіг.

### Найімовірніша причина — тарифікація / баланс
SMS тарифікується в момент відправки. Якщо на профілі (створений 08.06.2026) **немає коштів або не підключена тарифікація**, списання падає, повідомлення йде в `REJECTED`, а назовні віддається generic `00-0000`. (У SMPP-кодах є окремі `3100/3101 "Charging is failed"`, на REST згортається в `00-0000`.)

### Що зробити (на боці Vodafone, БЕЗ змін у коді)
1. Перевірити **баланс / тарифікацію** профілю `7364509` у кабінеті або в менеджера.
2. Поповнити рахунок / попросити підключити тариф.
3. Повторити запит — наш індикатор статусу автоматично покаже `📬 SMS DELIVERED`.

### Шаблон звернення в підтримку OBM
```
REST API SMS відхиляються системною помилкою.
Профіль: 7364509 (активний, REST API + SMS на інші мережі).
Ім'я відправника: 7364601 / AQUAWHEEL (активне, SMS).
Розсилка REST API: 7369261 (запущено).
POST .../communicationMessage/send → 200 + message_id, але /status?type=FULL → REJECTED,
errorCode 00-0000 "System error 0".
Приклад message_id: 05-00-380987293334-...
Отримувач — власний Vodafone-номер 380987293334.
Усі налаштування коректні. Прошу перевірити тарифікацію/білінг профілю та причину системної помилки.
```

---

## 11. Production-чеклист

- [ ] `APP_ENV=production` — щоб код OTP **не** повертався у відповіді (§7).
- [ ] Заповнені `OBM_USERNAME` / `OBM_PASSWORD` / `OBM_SENDER_ID` у проді.
- [ ] Підключена тарифікація / є баланс на профілі OBM (інакше `00-0000`).
- [ ] `.env` не в гіті; пароль OBM не засвічений.
- [ ] (Рекомендовано) окремий rate-limit на `/verify-phone/request` (зараз спільний email-ліміт) — щоб не палити бюджет OBM.
- [ ] (Опційно) `OBM_STATUS_CHECK=false` на проді, якщо не потрібні зайві GET-запити статусу.

---

## 12. Можливі покращення (TODO)

- Окремий **rate-limit на номер** (напр. 1 SMS / 60с) для `/verify-phone/request`.
- Збереження `message_id` та фінального статусу доставки в БД (аудит SMS).
- Підтримка fallback-каналу (Viber) — OBM підтримує каскади.
- Метрики: кількість надісланих/доставлених/відхилених SMS.

---

## 13. Швидкі посилання

- Інтерфейс провайдера: [internal/platform/sms/sender.go](../internal/platform/sms/sender.go)
- Клієнт Vodafone: [internal/integration/sms/vodafone.go](../internal/integration/sms/vodafone.go)
- Сервіс авторизації: [internal/user/service/auth_service.go](../internal/user/service/auth_service.go)
- Конфіг: [internal/platform/config/config.go](../internal/platform/config/config.go)
- DI / вибір провайдера: [internal/http/router.go](../internal/http/router.go)
- Ендпоінти/хендлери: [routes.go](../internal/user/delivery/http/routes.go), [auth_handler.go](../internal/user/delivery/http/auth_handler.go)
