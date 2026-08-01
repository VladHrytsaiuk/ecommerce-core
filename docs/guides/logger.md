# 📋 Uber-go/Zap – Логування у AquaWheel Store Backend

В AquaWheel Store Backend для логування використовується бібліотека **Uber-go/Zap**.  
Вона надає структуроване, швидке і типоване логування у вигляді JSON‑подібних записів, що зручно аналізувати у продакшен‑середовищі.

***

## Чому використовуємо Zap, а не log/stdlog

- **Структуроване логування** — поля виводяться як ключ‑значення, що дозволяє парсити їх інструментами (ELK, Loki, Grafana тощо).
- **Висока швидкість** — Zap використовує буферизацію та специфічні типи, щоб мінімізувати алокації.
- **Рівні логування** — `Debug`, `Info`, `Warn`, `Error`, `Fatal` тощо.
- **Гнучка конфігурація** — формат виводу (JSON/text), об’єднання/загорнення полів, енкодери.

***

## Як логер налаштований у нашому проєкті

У проєкті логер налаштовується у `internal/platform/logger` і отримує параметри із конфігурації:

```go
type Config struct {
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"` // "json" або "text"
}
```

Доступний логер експортується як інтерфейс `Logger`, який використовується повсюдно:

```go
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}
```

Конфігурацію із `.env` парсить `platform/config`, а потім передає в `platform/logger.New()`.

***

## Як використовувати Zap у коді

### 1. Отримання логера

Усередині кожного домену/сервісу логер передається через інтерфейс, наприклад:

```go
type ProductService struct {
    repo   product.Repository
    logger Logger
}
```

### 2. Прості повідомлення

```go
// Info
logger.Info("product created", 
    zap.String("id", "123"),
    zap.String("name", "Water tank"),
)

// Error
logger.Error("failed to create product",
    zap.Error(err),
    zap.String("request_id", reqID),
)
```

Запис в JSON матиме вигляд:

```json
{
  "level": "info",
  "msg": "product created",
  "id": "123",
  "name": "Water tank"
}
```

### 3. Готуємося до контекстного логування

Для HTTP‑запитів ми використовуємо **middleware**, яке додає до кожного request‑логів:

- `request_id` — унікальний ідентифікатор;
- `method` — HTTP‑метод (`GET`, `POST`);
- `url` — запитаний шлях;
- `duration` — час виконання;
- `status` — HTTP‑статус.

Приклад в middleware:

```go
logger.Info("request handled",
    zap.String("method", c.Request.Method),
    zap.String("url", c.Request.URL.Path),
    zap.Duration("duration", time.Since(start)),
    zap.Int("status", c.Writer.Status()),
    zap.String("request_id", rid),
)
```

***

## Рівні логування

| Рівень  | Коли використовувати |
|--------|----------------------|
| **Debug** | Детальні відомості для розробки та налагодження. У продакшені, як правило, вимкнені. |
| **Info**  | Загальні події, що показують хід роботи (`Server started`, `Product saved` тощо). |
| **Warn**  | Потенційно проблемні ситуації, що не вважаються помилками, але варті контролю (`rate limit`, `slow DB` тощо). |
| **Error** | Помилки, що завершилися невдало, але процес залишається жити (`invalid input`, `db err` тощо). |
| **Fatal** | Критичні помилки, що призводять до завершення процесу (`cannot connect to DB on startup`). |

Приклад:

```go
logger.Debugf("checking user permissions for user_id=%s", userID)
logger.Warnf("order is old, consider cleanup, order_id=%s", orderID)
logger.Errorf("payment failed: %v", err)
```

***

## Логування помилок та контексту

Zap має вбудовану підтримку помилок через `zap.Error(err)`.  
Рекомендований спосіб:

```go
if err != nil {
    logger.Error("failed to update order",
        zap.String("order_id", oid),
        zap.Error(err),
    )
    return err
}
```

Не рекомендується:

```go
// Погано: втрачає структуру та тип
logger.Error(fmt.Sprintf("failed to update order: %v", err))
```

***

## Загорнення полів через `With` / `Named` логерів

Для доменів, де потрібно додати спільний контекст у багато логів, використовується `With`:

```go
requestLogger := logger.With(
    zap.String("user_id", userID),
    zap.String("request_id", reqID),
)

// Всі наступні логи матимуть ці поля
requestLogger.Info("processing order")
```

***

## Конфігурація формату виводу

За допомогою конфігурації можна перемикатися між:

- **JSON** — зручно для продакшен‑логів, аналізу.
- **Text** — зручніше для локального дебагу.

Приклад конфігурації в `.env`:

```env
LOG_LEVEL=debug
LOG_FORMAT=json
```

У коді:

```go
cfg := zap.NewProductionConfig()
if cfg.LogLevel == "debug" {
    cfg = zap.NewDevelopmentConfig()
}
logger, _ := cfg.Build()
```

***

## Рекомендації з використання Zap

- Не використовуйте `fmt`‑рядки в `zap.Any`/`zap.String` для структурованих значень — використовуйте спеціальні `zap.Field` (`zap.String`, `zap.Int`, `zap.Bool`, `zap.Error`).
- Додавайте контекст (ID, correlation_id, status), але не секретні данные (паролі, токени).
- Логуйте входи в додаткові інтеграції (LiqPay, Нова Пошта):
    - старт запиту;
    - відповідь (у форматі `status`, `duration`, `error`).
- Використовуйте `logger.Named("domain")`, щоб розділити логі й швидко фільтрувати по домену.

***

## Як дивитися логи у Docker‑середовищі

Після запуску через `docker-compose up` логи можно переглянути так:

```bash
docker logs aquawheel-backend -f
```

Якщо використовується `LOG_FORMAT=json`, можна підключити:

```json
{
  "level": "info",
  "ts": "2026-03-30T12:00:00Z",
  "msg": "order processed",
  "order_id": "123",
  "status": "paid"
}
```

до ELK/Fluentd/Loki, щоб фільтрувати, агрегувати й створювати графіки.

***

## Короткий чек‑лист

- Використовуємо Zap → структуроване логування у JSON.
- Рівні `Debug/Info/Warn/Error/Fatal` → відповідні до типу події.
- Помилки → тільки через `zap.Error(err)`.
- Контекст (ID, статус, тривалість) → додається до кожного важливого логу.
- На продакшені → `LOG_LEVEL=info` або `warn`, локально — `debug`.
- Логи → виводяться в Docker‑контейнери і можуть бути зібрані зовнішніми системами.
