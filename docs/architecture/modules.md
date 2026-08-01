# Архітектура та опис модулів (Domain Modules)

Проєкт `AquaWheel Store` використовує архітектуру у стилі **Modular Monolith**, де весь код розділений на незалежні бізнес-домени (модулі) всередині папки `internal/`. Кожен модуль інкапсулює свою бізнес-логіку та взаємодіє з іншими через чітко визначені інтерфейси-сервіси (Dependency Injection).

Кожен модуль зазвичай складається з таких шарів:
1. `delivery/http` (напр., `cart_handler.go`) — HTTP Handlers, що відповідають за парсинг запиту та формування відповіді.
2. `service` (напр., `cart_service.go`) — Бізнес-логіка модуля (не знає про фреймворки чи специфіку БД).
3. `repository/postgres` (напр., `cart_repository.go`) — Реалізація сховища даних на базі PostgreSQL + GORM.
4. `domain` — Інтерфейси (напр., `CartRepository`) та сутності.

Нижче наведено детальний огляд ключових модулів.

---

## 1. User & Auth (`internal/user`)

Відповідає за безпеку, ідентифікацію клієнтів та збереження даних базового користувача. Таблиці в БД: `users`, `sessions`, `verify_codes`, `user_addresses`.

### Основний функціонал:
- **Реєстрація та логін:** Управління створенням акаунтів через `AuthService`.
- **Сесії та JWT:** При вході в `AuthHandler.Login` генерується JWT-токен (`token.Maker`) та створюється запис у таблиці `sessions` (з `refresh_token`), що дозволяє керувати "живими" сесіями користувачів.
- **Підтвердження Email:** Реалізовано за допомогою генерації кодів доступу (зберігаються в таблиці `verify_codes` через `verifyCodeRepo`) та надсилання листів.
- **WebSocket Hub:** Для підтвердження реєстрації в реальному часі система використовує `notification.Hub` (в `internal/platform/notification`). Клієнт підключається до сокету (`/api/auth/ws`), і коли код валідується, хаб викликає `NotifyUser(userID, data)`, пушачи JSON клієнту.
- **Background Worker:** `CleanupWorker` (файл `internal/user/service/cleanup_worker.go`) запускається з інтервалом у `main.go`. Воркер щоденно видаляє старі сесії та коди підтвердження з бази: `DeleteExpiredSessions` та `DeleteExpired`.

---

## 2. Catalog: Product & Category (`internal/product`, `internal/category`)

Серце магазину, яке обробляє каталогізацію товарів. Таблиці: `products`, `categories`, `brands`, `product_images` тощо.

### Особливості реалізації:
- **Окремі сутності:** Продукти, Категорії, Бренди (Brands) відокремлені у свої простори.
- **Багатомовність (Localization):** Підтримується локалізація контенту (наприклад, описи мовами `uk` та `en`). Локаль обробляється за допомогою `middleware.LocaleMiddleware()`, яка витягує код мови з URL (наприклад: `/api/uk/products`). Ця мова потім передається до репозиторію (через контекст або DTO) для фільтрації JSONB полів перекладу (якщо застосовується).
- **Пагінація та Сортування:** На рівні HTTP запитів використовується структура `pagination.Params` (`internal/shared/pagination`), яка парсить `?page=1&limit=20`. Далі ця конфігурація застосовується у репозиторіях (через `Limit()` та `Offset()`). Відповідь загортається у `pagination.PagedResponse` з метаданими.

---

## 3. Cart & Wishlist (`internal/cart`, `internal/wishlist`)

Ці два модулі мають схожу технічну архітектуру, спрямовану на покращення зручності користувачів (дозволяють додавати в кошик та вішліст без реєстрації). Таблиці: `carts`, `cart_items`, `wishlists`, `wishlist_items`.

### Ключові концепції:
- **Анонімні сесії:** Користувач-гість ідентифікується за `session_id`, який зберігається в куці `X-Session-ID` за допомогою `middleware.OptionalAuthMiddleware` + `middleware.SessionMiddleware`. У таблицях `carts` та `wishlists` є колонка `session_id` (UUID), яка використовується замість `user_id`.
- **Merged on Login:** Якщо анонімний користувач додав товари, а потім авторизувався, система "мержить" (об'єднує) гостьовий кошик/вішліст з основним (за `user_id`) у методі `MergeAnonymousCart` / `MergeAnonymousWishlist`.
- **Очищення сміття:** Для того, щоб анонімні кошики не захаращували базу, працюють воркери:
  - `CartCleanupWorker` (файл `internal/cart/service/cleanup_worker.go`) має параметр `maxAge = 14 днів` (14 * 24h). Він викликає `repo.DeleteExpiredAnonymous`.
  - `WishlistCleanupWorker` робить те ж саме для списку бажань.

---

## 4. Shipment (`internal/shipment`)

Модуль для роботи з логістикою та провайдерами доставки. Таблиці: `shipping_rules`.

### Організація:
- Реалізовано через патерн "Стратегія": існує інтерфейс `shipmentDomain.ShipmentProvider`, і конкретна реалізація у `internal/shipment/provider/novaposhta`.
- Клієнт Нової Пошти `novaposhta.NewClient()` конфігурується ключем `NovaPoshtaAPIKey`.
- Всі провайдери реєструються в мапі `map[string]shipmentDomain.ShipmentProvider` в `router.go` та передаються до `ShipmentService`.
- Це дозволяє розраховувати вартість доставки через `ShipmentService.CalculateShippingCost` з використанням зовнішніх API та локальних `ShippingRule` (фіксована ціна, безкоштовна доставка та ін.).

---

## 5. Інші модулі

- **Order (`internal/order`)**: Робота з життєвим циклом замовлення, оформлення покупок, інтеграція доставки.
- **Payment (`internal/payment`)**: Перевірка статусів платежів, управління транзакціями (включає інтеграцію вебхуків LiqPay).
- **Discount (`internal/discount`)**: Модуль управління знижками та промокодами.
- **Inventory (`internal/inventory`)**: Облік залишків на складах (перевірка стоку перед оформленням замовлення).
- **Sitemap (`internal/sitemap`) & Redirect (`internal/redirect`)**: Динамічна генерація sitemap та управління історією посилань (SEO).
- **Integration (`internal/integration`)**: Зовнішні провайдери: Нова Пошта, Укрпошта, LiqPay, SMS (Vodafone).
