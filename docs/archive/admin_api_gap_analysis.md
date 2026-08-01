# 🔍 Аналіз покриття Admin API — AquaWheel Store

Було проведено детальний аналіз поточної кодової бази проєкту у гілці `feature/admin-setup` (зокрема файлу конфігурації роутера [router.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/http/router.go), файлів маршрутизації доменних модулів та документації [api_analysis.md](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/docs/infra/api_analysis.md) та [admin_product_crud.md](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/docs/admin_product_crud.md)).

Нижче наведено детальний звіт про те, які адміністративні ендпоінти вже реалізовано, які повністю відсутні через неімплементовані модулі, та що рекомендується додати для отримання повноцінної адмін-панелі.

---

## 📈 Поточний статус покриття Admin API

Згідно з розділом **"Необхідні Admin API"** у файлі [api_analysis.md](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/docs/infra/api_analysis.md#L216-L234), заплановано **12 основних адміністративних ендпоінтів**.

### 🟢 1. Повністю реалізовані модулі (Catalog & Auth)

Вся бізнес-логіка каталогу товарів (Products, Categories, Brands, Attributes, Badges) та аутентифікації адміна є **повністю готовою** та відповідає Clean Architecture.

*   **Auth (`internal/user`)**:
    *   `POST /api/admin/auth/login` — Вхід для адміністратора (тільки роль `admin`). [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/user/delivery/http/routes.go#L34)
    *   `POST /api/admin/auth/refresh` — Оновлення JWT токенів для адмін-сесії. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/user/delivery/http/routes.go#L35)
*   **Products (`internal/product`)**:
    *   `POST /api/admin/products` — Створення нового товару (з перекладами, варіаціями, атрибутами та фото). [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L23)
    *   `PATCH /api/admin/products/:id` — Часткове оновлення товару (з Full Sync для варіацій/атрибутів). [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L24)
    *   `DELETE /api/admin/products/:id` — М'яке видалення (Soft Delete) товару. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L25)
    *   `PATCH /api/admin/reviews/:id/approve` — Модерація (схвалення) відгуків. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L26)
*   **Image Management (`internal/product`)**:
    *   `POST /api/admin/products/:id/images` — Завантаження нових фото до існуючого товару. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L29)
    *   `DELETE /api/admin/products/:id/images/:img_id` — Видалення конкретного фото. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L30)
    *   `PATCH /api/admin/products/:id/images/:img_id` — Зміна метаданих фото (primary, hover, варіація). [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L31)
    *   `PATCH /api/admin/products/:id/images/reorder` — Зміна порядку відображення фотографій. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L32)
*   **Categories (`internal/category`)**:
    *   `POST /api/admin/categories` — Створення категорії. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/category/delivery/http/routes.go#L16)
    *   `PATCH /api/admin/categories/:id` — Оновлення категорії. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/category/delivery/http/routes.go#L17)
    *   `DELETE /api/admin/categories/:id` — Видалення категорії. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/category/delivery/http/routes.go#L18)
*   **Brands (`internal/product`)**:
    *   `POST /api/admin/brands`, `PATCH /api/admin/brands/:id`, `DELETE /api/admin/brands/:id` — Керування брендами. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L42-L44)
*   **Attributes (`internal/product`)**:
    *   `POST /api/admin/attributes`, `PATCH /api/admin/attributes/:id`, `DELETE /api/admin/attributes/:id`, `PATCH /api/admin/attributes/reorder` — Керування атрибутами (Drag & Drop сортування). [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L54-L57)
*   **Badges (`internal/product`)**:
    *   `GET /api/admin/badges`, `POST /api/admin/badges`, `PATCH /api/admin/badges/:id`, `DELETE /api/admin/badges/:id` — Керування маркетинговими бейджами товарів. [routes.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/routes.go#L66-L69)

---

### 🔴 2. Повністю відсутні модулі (Orders, Users Management, Payments)

Ці модулі наразі містять лише заглушки `.gitkeep` у теках `internal/order`, `internal/payment`, `internal/customer`. Через це відповідні адмін-ендпоінти **не реалізовані взагалі**:

1.  **Керування замовленнями (Orders)**:
    *   `GET /api/admin/orders` — Список замовлень (канбан) ❌ **Відсутній**
    *   `PATCH /api/admin/orders/:id/status` — Зміна статусу замовлення ❌ **Відсутній**
    *   `PATCH /api/admin/orders/:id/delivery` — Додавання ТТН (Нова Пошта) ❌ **Відсутній**
2.  **Керування користувачами (Users)**:
    *   `GET /api/admin/users` — Список користувачів ❌ **Відсутній**
    *   `PATCH /api/admin/users/:id/block` — Блокування користувача ❌ **Відсутній**

---

### 📝 3. Важливе уточнення щодо варіацій товарів (SKU)

У файлі [api_analysis.md](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/docs/infra/api_analysis.md#L225) зазначено ендпоінт:
*   `POST /admin/products/:id/variations` — Додати варіацію (SKU).

У коді окремого ендпоінту **немає**. Натомість, відповідно до [admin_product_crud.md](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/docs/admin_product_crud.md#L71-L80), керування варіаціями повністю винесено всередину **Full Sync** логіки створення (`POST /api/admin/products`) та оновлення (`PATCH /api/admin/products/:id`) товару.
> **Це набагато зручніший підхід для фронтенду**, оскільки дозволяє редагувати весь товар та його SKU на одній сторінці форми в адмінці за один запит. Окремий ендпоінт для додавання SKU не є обов'язковим, але за потреби його можна винести.

---

## 📋 Зведена таблиця аналізу покриття ендпоінтів для адмінки

| # | Метод | Ендпоінт | Опис | Пріоритет | Стан у коді | Деталі / Файли |
|---|---|---|---|---|---|---|
| 1 | `POST` | `/api/admin/auth/login` | Вхід адміністратора | P0 🔴 | **Реалізовано** ✅ | [auth_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/user/delivery/http/auth_handler.go) |
| 2 | `POST` | `/api/admin/auth/refresh` | Оновлення JWT токенів | P0 🔴 | **Реалізовано** ✅ | [auth_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/user/delivery/http/auth_handler.go) |
| 3 | `POST` | `/api/admin/products` | Створити товар | P1 🟡 | **Реалізовано** ✅ | [product_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/product_handler.go) |
| 4 | `PATCH` | `/api/admin/products/:id` | Оновити товар | P1 🟡 | **Реалізовано** ✅ | [product_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/product_handler.go) |
| 5 | `DELETE` | `/api/admin/products/:id` | Деактивувати/видалити товар | P1 🟡 | **Реалізовано** ✅ | [product_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/product_handler.go) |
| 6 | `POST` | `/api/admin/products/:id/variations` | Додати варіацію (SKU) | P1 🟡 | **Реалізовано в PATCH/POST товару** (Full Sync) | Окремого шляху немає, керується через масив `variations` у `payload` продукту. |
| 7 | `POST` | `/api/admin/products/:id/images` | Завантажити фото товару | P1 🟡 | **Реалізовано** ✅ | [product_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/product_handler.go) |
| 8 | `POST` | `/api/admin/categories` | Створити категорію | P1 🟡 | **Реалізовано** ✅ | [category_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/category/delivery/http/category_handler.go) |
| 9 | `PATCH` | `/api/admin/orders/:id/status` | Змінити статус замовлення | P1 🟡 | **Відсутній** ❌ | Модуль `internal/order` не реалізовано. |
| 10 | `GET` | `/api/admin/orders` | Список замовлень | P1 🟡 | **Відсутній** ❌ | Модуль `internal/order` не реалізовано. |
| 11 | `PATCH` | `/api/admin/orders/:id/delivery` | Додати ТТН (Нова Пошта) | P1 🟡 | **Відсутній** ❌ | Модуль `internal/order` не реалізовано. |
| 12 | `GET` | `/api/admin/users` | Список користувачів | P2 🟢 | **Відсутній** ❌ | Тільки заглушка, немає хендлера. |
| 13 | `PATCH` | `/api/admin/users/:id/block` | Заблокувати користувача | P2 🟢 | **Відсутній** ❌ | Не реалізовано у модулі `user`. |
| 14 | `PATCH` | `/api/admin/reviews/:id/approve` | Модерація відгуків | P2 🟢 | **Реалізовано** ✅ | [product_handler.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/product/delivery/http/product_handler.go) |

---

## 💡 Що варто додати/розширити для повноцінної адмінки?

Окрім базових ендпоінтів із таблиці, для побудови преміальної, професійної адмін-панелі (CRM) AquaWheel вкрай рекомендується реалізувати наступний функціонал:

### 1. 📊 Панель аналітики та показників (Dashboard API)
Адміністратору критично важливо бачити поточний стан бізнесу при першому вході в систему.
*   `GET /api/admin/dashboard/stats` — Загальна сума продажів за період, кількість активних замовлень, кількість нових користувачів, товари з критично низьким залишком на складі.
*   `GET /api/admin/dashboard/charts` — Дані для графіків продажів (денні/тижневі/місячні зрізи).

### 🛡 2. Перегляд журналу дій адмінів (Audit Logs API)
У проєкті вже є потужна асинхронна система аудиту та маскування чутливих даних ([audit.go](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/audit/domain/audit.go#L10-L28)), яка записує кожну мутуючу дію адміністратора. Проте **немає жодного ендпоінту, щоб ці логи переглянути**.
*   `GET /api/admin/audit-logs` — Перегляд списку логів аудиту з пагінацією та фільтрацією (по адміну, шляху, статусу чи даті) для безпеки та контролю персоналу.

### 🏷 3. Модуль знижок та промокодів (Discount & Coupons)
Тека `internal/discount` порожня. Маркетинг та гнучке ціноутворення є основою будь-якого e-commerce.
*   `GET /api/admin/discounts` — Список акцій/купонів.
*   `POST /api/admin/discounts`, `PATCH /api/admin/discounts/:id`, `DELETE /api/admin/discounts/:id` — CRUD для створення промокодів (наприклад, `-10%`, фіксована знижка, безкоштовна доставка) та прив'язки акцій до категорій чи конкретних товарів.

### 📦 4. Інвентаризація та Склад (Inventory Management)
Тека `internal/inventory` порожня. Оновлювати залишки товарів лише через загальний PATCH продукту не завжди зручно, коли склад змінюється швидко.
*   `GET /api/admin/inventory/low-stock` — Швидкий перегляд товарів, що закінчуються.
*   `PATCH /api/admin/inventory/stock` — Масове швидке оновлення кількості одиниць на складі для конкретних SKU (без необхідності завантажувати всю форму товару).

### 🚚 5. Керування правилами доставки (Shipping Rules)
У модулі доставки є репозиторій `ShippingRuleRepository` ([router.go:L142](file:///Users/Vlad/Documents/Work/AquaWheel/AquawheelStore_BACKEND/internal/http/router.go#L142)), але немає адміністративних API для їх налаштування (наприклад, встановлення порогу безкоштовної доставки: "при замовленні від 2000 грн доставка безкоштовна").
*   `GET /api/admin/delivery/rules` та `PUT /api/admin/delivery/rules` — Налаштування правил вартості доставки.

---

## 🎯 Наступні кроки для реалізації (Рекомендація)

1.  **Пріоритет №1**: Почати імплементацію модуля **`internal/order`** (База даних, моделі, репозиторії GORM, хендлери) — це розблокує як клієнтське оформлення замовлень, так і адмінські ендпоінти для перегляду списку замовлень (канбан) та зміни статусів.
2.  **Пріоритет №2**: Додати хендлери для керування користувачами (`GET /api/admin/users`, `PATCH /api/admin/users/:id/block`), використовуючи вже готову структуру бази даних у `internal/user`.
3.  **Пріоритет №3**: Створити простий ендпоінт для перегляду журналу аудиту (`GET /api/admin/audit-logs`), оскільки інфраструктура для збору цих логів уже повністю готова та працює!
