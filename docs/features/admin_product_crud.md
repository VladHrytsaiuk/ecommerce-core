# 📦 Документація: Admin Catalog CRUD API

Ця документація описує принципи роботи з API для керування товарами, категоріями, атрибутами та брендами в панелі адміністратора AquaWheel.

> [!IMPORTANT]  
> Усі адміністративні методи вимагають авторизації через заголовок `Authorization: Bearer <token>`.

---
## 1. 🛍 Керування товарами (Products)


### 🚀 Ендпоінти

| Метод | Шлях | Опис |
| :--- | :--- | :--- |
| **POST** | `/api/admin/products` | Створення нового товару (з перекладами, варіаціями, атрибутами та фотографіями) |
| **PATCH** | `/api/admin/products/{id}` | Часткове оновлення товару (Partial Update). Підтримує Full Sync для варіацій/атрибутів. |
| **DELETE** | `/api/admin/products/{id}` | М'яке видалення (Soft Delete) товару |
| **POST** | `/api/admin/products/{id}/images` | Завантаження нових фотографій до існуючого товару |
| **PATCH** | `/api/admin/products/{id}/images/{img_id}`| Зміна ролі фото (primary, hover), сортування або прив'язки до варіації |
| **DELETE** | `/api/admin/products/{id}/images/{img_id}`| Видалення конкретного фото товару |
| **PATCH** | `/api/admin/products/{id}/images/reorder` | Масове оновлення порядку фотографій (`sort_order`) |

### 🛠 Формат запиту для створення/оновлення товару (Multipart Form)

Створення та оновлення товару використовує **`multipart/form-data`**. Запит складається з:
1. **`payload`** — текстове поле, що містить весь JSON з даними товару.
2. **Файли зображень** — передаються як окремі поля з довільними іменами ключів (напр., `my_file`, `photo_v1`).

#### 🔗 Як працює мапінг зображень (images_meta)
Замість того, щоб вгадувати роль фото за складною назвою ключа (як було раніше), ви явно описуєте кожен файл у масиві `images_meta` всередині `payload`.

| Поле в JSON | Опис |
| :--- | :--- |
| **`key`** | **Обов'язково.** Має точно збігатися з ім'ям поля (key) у Multipart Form. |
| **`is_primary`** | `true`, якщо це головне фото (відображається першим у списку). |
| **`is_hover`** | `true`, якщо це фото для ефекту наведення (hover). |
| **`sort_order`** | Число для ручного сортування. |
| **`variation_id`**| UUID варіації, до якої кріпиться фото. Якщо `null` — це загальне фото товару. |
| **`badge_ids`**   | Масив ID бейджів (int). Можна призначати як всьому товару, так і окремій варіації. |

### 🚀 Приклад створення товару (Повний запит)

Щоб протестувати **POST** `/api/admin/products`, у Postman оберіть `form-data`:
1. Поле **`payload`** (тип Text):
```json
{
  "brand_id": "d1b7a6c9-e8f0-4d5a-bcde-1234567890ab",
  "category_id": "7ec3a1d3-3bdf-4d5a-8b8a-1a8a2c69d8a1",
  "is_active": true,
  "name_uk": "Мило рідке",
  "variations": [
    { "id": "b1a1c1d1-e1f1-4121-a1b1-c1d1e1f1a1b1", "sku": "SOAP-1", "price": 5500, "quantity_value": 1 }
  ],
  "images_meta": [
    { "key": "f_main", "is_primary": true, "variation_id": null },
    { "key": "f_var", "is_primary": true, "variation_id": "b1a1c1d1-e1f1-4121-a1b1-c1d1e1f1a1b1" }
  ]
}
```
2. Поле **`f_main`** (тип File): (ваше фото)
3. Поле **`f_var`** (тип File): (ваше фото)

#### 🏷 Керування бейджами
При створенні або оновленні ви можете передати масив `badge_ids`:
- На рівні товару: `"badge_ids": [1, 2]`
- На рівні варіації: `{"id": "uuid", "badge_ids": [3]}`

Детальніше про логіку бейджів читайте у [**Product Badges Guide**](PRODUCT_BADGES.md).

> [!WARNING]  
> **Оновлення товару (PATCH):** 
> 1. Всі поля в JSON є опціональними. Передавайте тільки те, що змінилося.
> 2. **Оновлення варіацій (`variations`):** Масив працює за принципом **Full Sync для списку**, але **Partial Update для полів**. 
>    - Якщо ви передаєте масив `variations`, у ньому мають бути **всі** варіації, які ви хочете залишити. Варіації, яких немає в масиві, будуть видалені.
>    - Для кожної варіації достатньо передати `id` та **тільки ті поля, які змінилися**. Наприклад, щоб змінити статус: `{"id": "uuid", "is_active": false}`. Інші поля (ціна, залишки) не затруться.
>    - Щоб не чіпати варіації взагалі, не відправляйте поле `variations`. Щоб видалити всі, передайте порожній масив `[]`.
> 3. **Атрибути (`attribute_values`):** Працюють за принципом **Full Sync**. Передавайте весь оновлений масив.
> 4. **Видалення фото:** Щоб видалити існуючі зображення при PATCH товару, додайте їх UUID до масиву `"images_to_delete": ["uuid1", "uuid2"]` у `payload`.

---

## 🧪 Приклади для тестування (cURL / Postman)

### 1. Сортування фото (Reorder Images)
**PATCH** `/api/admin/products/{product_id}/images/reorder`  
Розставляє `sort_order` у базі згідно з порядком ID у масиві (перший у масиві = перший на сайті).

**cURL:**
```bash
curl -X PATCH http://localhost:8080/api/admin/products/{id}/images/reorder \
     -H "Authorization: Bearer YOUR_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"ids": ["uuid-фото-1", "uuid-фото-2"]}'
```

### 2. Оновлення метаданих конкретного фото
**PATCH** `/api/admin/products/{id}/images/{img_id}`

**cURL:**
```bash
curl -X PATCH http://localhost:8080/api/admin/products/{id}/images/{img_id} \
     -H "Authorization: Bearer YOUR_TOKEN" \
     -d '{"is_primary": true, "sort_order": 5}'
```

### 3. Довантаження нових фото до існуючого товару
**POST** `/api/admin/products/{id}/images`  

**cURL:**
```bash
curl -X POST http://localhost:8080/api/admin/products/{id}/images \
     -H "Authorization: Bearer YOUR_TOKEN" \
     -F 'payload=[{"key": "new_pic", "is_primary": false, "sort_order": 10}]' \
     -F 'new_pic=@/path/to/image.jpg'
```

### 4. Сортування атрибутів (Reorder Attributes)
**PATCH** `/api/admin/attributes/reorder`  

**cURL:**
```bash
curl -X PATCH http://localhost:8080/api/admin/attributes/reorder \
     -H "Authorization: Bearer YOUR_TOKEN" \
     -d '{"ids": [5, 3, 1]}'
```

---

## 2. 🗂 Керування категоріями (Categories)

| Метод | Шлях | Опис |
| :--- | :--- | :--- |
| **POST** | `/api/admin/categories` | Створення категорії |
| **PATCH** | `/api/admin/categories/{id}` | Оновлення категорії |
| **DELETE** | `/api/admin/categories/{id}` | Видалення категорії |

**Формат JSON (application/json) для POST:**
```json
{
  "parent_id": null,
  "name_uk": "Нова категорія",
  "name_en": "New Category"
}
```
*Для PATCH запиту всі поля опціональні (`name_uk`, `name_en`, `parent_id`).*

---

## 3. 🏷 Керування атрибутами (Attributes)

| Метод | Шлях | Опис |
| :--- | :--- | :--- |
| **POST** | `/api/admin/attributes` | Створення атрибута |
| **PATCH** | `/api/admin/attributes/{id}` | Оновлення атрибута |
| **DELETE** | `/api/admin/attributes/{id}` | Видалення атрибута |
| **PATCH** | `/api/admin/attributes/reorder` | Зміна порядку атрибутів (Drag & Drop) |

**Формат JSON для створення (application/json):**
```json
{
  "code": "color",
  "name_uk": "Колір",
  "name_en": "Color",
  "sort_order": 1,
  "is_filterable": true,
  "is_variant_specific": false,
  "unit_id": null
}
```

**Формат для масової зміни порядку (PATCH `/reorder`):**
```json
{
  "ids": [5, 3, 1, 4, 2]
}
```
*Масив `ids` має містити ID атрибутів (int) у новому бажаному порядку.*

---

## 4. 🏢 Керування брендами (Brands)

| Метод | Шлях | Опис |
| :--- | :--- | :--- |
| **POST** | `/api/admin/brands` | Створення бренду |
| **PATCH** | `/api/admin/brands/{id}` | Оновлення бренду |
| **DELETE** | `/api/admin/brands/{id}` | Видалення бренду |

**Формат JSON (application/json):**
```json
{
  "name": "Назва бренду"
}
```

---

## 🎨 Логіка відображення на Фронтенді

Зображення в API (`GET /api/uk/products/{id}`) розділені:
*   `res.images` — спільні фото товару.
*   `res.variations[].images` — фото конкретної варіації.

**Алгоритм вибору фото для картки товару/варіації:**
1. Перевірити, чи є у вибраної варіації фото з `is_primary: true`.
2. Якщо немає — взяти фото з `is_primary: true` із загального списку `res.images`.
3. Якщо і там немає — взяти перше доступне загальне фото.

---

## ⚙️ Особливості бекенду (Корисна інформація)

* **Cloudinary PublicID:** Бекенд самостійно обрізає розширення файлу (напр. `.webp`) перед збереженням в Cloudinary, тому фронтенд отримує чисті URL-адреси.
* **Ціни:** Передаються як цілі числа в копійках (`10.50 грн = 1050`).
* **Мови:** Мультимовність реалізована через "плоску" структуру з суфіксами `_uk` та `_en`. Якщо `name_en` порожнє, система автоматично використовує значення з `name_uk`.
* **JSONB у Postgres:** Атрибути товару зберігаються у форматі `JSONB`, що дозволяє швидку фільтрацію та зберігання числових і рядкових значень без конфліктів.

---

## 5. 🔐 Адміністративна авторизація (Admin Auth)

Адмін-панель має **окремий потік авторизації**, відділений від клієнтського. Адміністратор не може увійти через `/api/auth/login` з правами адміна, і клієнт не може увійти через `/api/admin/auth/login`.

### 🚀 Ендпоінти

| Метод | Шлях | Опис |
| :--- | :--- | :--- |
| **POST** | `/api/admin/auth/login` | Вхід для адміністратора (тільки роль `admin`) |
| **POST** | `/api/admin/auth/refresh` | Оновлення JWT токенів для адмін-сесії |

> [!NOTE]
> Адміністратор може входити і як клієнт через `/api/auth/login` (для тестування магазину), але клієнт **не може** авторизуватися як адміністратор.

### 📝 Формат запиту

**POST `/api/admin/auth/login`** — `application/json`:
```json
{
  "email": "admin@example.com",
  "password": "securePassword123"
}
```

**Відповідь (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": "uuid",
    "email": "admin@example.com",
    "role_id": 2,
    "first_name": "Admin",
    "last_name": "User"
  }
}
```

**POST `/api/admin/auth/refresh`** — `application/json`:
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

### 🧪 cURL приклади

```bash
# Логін адміністратора
curl -X POST http://localhost:8080/api/admin/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email": "admin@example.com", "password": "securePassword123"}'

# Оновлення токенів
curl -X POST http://localhost:8080/api/admin/auth/refresh \
     -H "Content-Type: application/json" \
     -d '{"refresh_token": "YOUR_REFRESH_TOKEN"}'
```

### 🔑 Ролі (RBAC)

| RoleID | Назва | Доступ |
| :---: | :--- | :--- |
| `1` | Customer | `/api/auth/*`, `/api/uk/*`, `/api/en/*` |
| `2` | Admin | Все, що має Customer + `/api/admin/*` |

Перевірка ролі відбувається на двох рівнях:
1. **AuthService.Login()** — при логіні перевіряється, чи `user.role_id` відповідає потрібній ролі.
2. **AdminMiddleware** — після JWT верифікації перевіряє `role_id` з контексту для кожного захищеного маршруту.

---

## 6. 📋 Журнал аудиту (Audit Trail)

Кожна мутуюча операція адміністратора (POST, PATCH, DELETE) автоматично логується в таблицю `audit_logs` у базі даних.

### Що записується

| Поле | Опис |
| :--- | :--- |
| `id` | UUID запису аудиту |
| `user_id` | UUID адміністратора, який виконав дію |
| `method` | HTTP метод (`POST`, `PATCH`, `DELETE`) |
| `path` | Шлях запиту (`/api/admin/products/uuid`) |
| `ip` | IP-адреса клієнта |
| `user_agent` | User-Agent браузера/клієнта |
| `status_code` | HTTP-код відповіді |
| `payload` | Тіло запиту (JSON, обмежене до 2KB) |
| `duration_ms` | Тривалість обробки запиту в мілісекундах |
| `created_at` | Час створення запису |

### 🛡 Маскування чутливих даних

Middleware автоматично замінює значення чутливих полів на `[REDACTED]` перед збереженням у лог. Маскуються поля:

- `password`, `new_password`, `old_password`
- `token`, `access_token`, `refresh_token`
- `secret`, `api_key`
- `cvv`, `card_number`

**Приклад:** Якщо адмін надіслав `{"email": "a@b.com", "password": "123"}`, у логах буде збережено:
```json
{"email": "a@b.com", "password": "[REDACTED]"}
```

### ⚡ Асинхронний запис

Аудит записується **асинхронно** в окремій горутині з таймаутом 5 секунд. Це гарантує, що:
1. Затримки/помилки запису аудиту **не впливають** на швидкість відповіді API.
2. У разі помилки БД — лог все одно зберігається у Zap (як fallback у stdout/файл).

### 📝 Multipart-запити

Для `multipart/form-data` запитів (створення/оновлення товарів) middleware логує тільки поле `payload` з JSON-даними, ігноруючи бінарні файли зображень. Це запобігає OOM-помилкам.

---

## 7. 🛡 Безпека та інфраструктура

### Rate Limiting (Обмеження запитів)

| Група маршрутів | Ліміт | Burst | Опис |
| :--- | :--- | :---: | :--- |
| `/api/admin/auth/*` | 1 req / 5 сек | 3 | Захист від перебору паролів |
| `/api/auth/*` (клієнтський) | 5 req / сек | 10 | Загальне обмеження |

Rate limiter працює **per-IP** і автоматично очищує записи неактивних IP кожну хвилину.

### Timeout Middleware

Кожен адмін-запит має таймаут **5 секунд**. Якщо обробка запиту перевищує цей ліміт, контекст скасовується і клієнт отримує помилку. Це запобігає зависанню API при проблемах з БД або Cloudinary.

### Soft Delete (М'яке видалення)

Видалення товарів, брендів, категорій та атрибутів **не видаляє** записи з бази даних. Замість цього встановлюється мітка `deleted_at`, а GORM автоматично фільтрує такі записи з усіх SELECT-запитів. Це дозволяє:
- Зберігати історію замовлень клієнтів
- Відновлювати випадково видалені товари (через пряме оновлення в БД)
- Зберігати цілісність зовнішніх ключів (FK)

