# 🔍 Документація: SEO-оптимізація (Мета-теги та Alt-тексти)

Цей посібник описує роботу з SEO-даними на бекенді AquaWheel: мета-тегами (title, description, keywords) для товарів/категорій та альтернативними текстами (alt) для зображень товарів.

---

## 📂 1. Архітектура збереження в БД

Усі SEO-дані локалізовані відповідно до обраної мови (`uk`, `en`).

### А. Мета-теги товарів та категорій
Мета-теги додані як текстові поля в таблиці перекладів. Це дозволяє мати унікальні SEO-параметри для кожної мовної версії:
*   **Категорії**: таблиця `category_translation`
    *   `meta_title` (VARCHAR)
    *   `meta_description` (TEXT)
    *   `meta_keywords` (TEXT)
*   **Товари**: таблиця `product_translation`
    *   `meta_title` (VARCHAR)
    *   `meta_description` (TEXT)
    *   `meta_keywords` (TEXT)

### Б. Alt-тексти для зображень товарів
Зображення зберігаються в таблиці `product_image`. Оскільки одне фото може використовуватися на обох мовних версіях сайту, альтернативний текст (`alt_text`) збережений у форматі **`JSONB`** вигляду:
```json
{
  "uk": "Опис фото українською",
  "en": "Image description in English"
}
```
Це дозволяє уникнути додаткових JOIN-таблиць та отримувати alt-тексти максимально швидко.

---

## 🛠 2. Керування через Admin API

### А. Категорії (`/api/admin/categories`)

Для створення чи оновлення категорій надсилайте стандартний `application/json`.

#### ➕ Створення категорії (`POST /api/admin/categories`)
**Тело запиту (JSON):**
```json
{
  "parent_id": null,
  "slug": "dlia-posudu",
  "name_uk": "Для посуду",
  "name_en": "Dishwashing",
  "meta_title_uk": "Купити засоби для миття посуду | AquaWheel",
  "meta_title_en": "Buy Dishwashing Products Online | AquaWheel",
  "meta_description_uk": "Найкращі екологічні засоби для миття посуду та посудомийних машин.",
  "meta_description_en": "Best eco-friendly dishwashing detergents and liquids.",
  "meta_keywords_uk": "мило, посуд, еко",
  "meta_keywords_en": "soap, dishwashing, eco"
}
```

#### ✏️ Оновлення категорії (`PATCH /api/admin/categories/{id}`)
Усі поля є опціональними. Передавайте тільки ті мета-теги, які хочете змінити:
```json
{
  "meta_title_uk": "Новий заголовок для посуду",
  "meta_description_uk": "Новий опис категорії українською."
}
```

---

### Б. Товари (`/api/admin/products`)

Для створення та оновлення товарів використовується **`multipart/form-data`**.
1. Текстові дані передаються як JSON-рядок у полі форми **`payload`**.
2. Файли зображень додаються окремими файлами.

#### ➕ Створення товару (`POST /api/admin/products`)
У `payload` ви можете передати як мета-теги товару, так і alt-тексти для нових зображень (через об'єкт `images_meta`):

**Поле форми `payload` (тип Text/JSON):**
```json
{
  "brand_id": "d1b7a6c9-e8f0-4d5a-bcde-1234567890ab",
  "category_id": "7ec3a1d3-3bdf-4d5a-8b8a-1a8a2c69d8a1",
  "is_active": true,
  "name_uk": "Таблетки для посудомийки Classic",
  "name_en": "Dishwasher Tablets Classic",
  "description_uk": "Опис...",
  "description_en": "Description...",
  
  "meta_title_uk": "Таблетки для посудомийних машин Classic купити",
  "meta_title_en": "Buy Dishwasher Tablets Classic Online",
  "meta_description_uk": "Екологічні таблетки для посудомийки. Ефективно видаляють жир.",
  "meta_description_en": "Eco dishwasher tablets. Removes grease effectively.",
  "meta_keywords_uk": "таблетки, посудомийка, еко",
  "meta_keywords_en": "tablets, dishwasher, eco",

  "variations": [
    {
      "sku": "TAB-CL-60",
      "price": 45000,
      "quantity_value": 60,
      "unit_id": 6
    }
  ],
  "images_meta": [
    {
      "key": "img_main",
      "is_primary": true,
      "sort_order": 1,
      "alt_text_uk": "Таблетки для посудомийки Classic 60 шт.",
      "alt_text_en": "Dishwasher Tablets Classic 60 pcs pack"
    }
  ]
}
```
**Поле форми `img_main` (тип File):** [обрати файл зображення]

#### ✏️ Оновлення товару (`PATCH /api/admin/products/{id}`)
Тут діє принцип **часткового оновлення**. Передавайте тільки ті SEO-поля, які змінилися:

**Поле форми `payload` (тип Text/JSON):**
```json
{
  "meta_title_uk": "Оновлений мета-заголовок",
  "meta_description_uk": "Оновлений мета-опис товару."
}
```

---

### В. Керування зображеннями товарів

Ви можете додавати зображення до вже існуючого товару або оновлювати метадані окремої фотографії.

#### ➕ Завантаження зображень до існуючого товару (`POST /api/admin/products/{id}/images`)
Використовує `multipart/form-data`.

*   Поле форми **`payload`** (опис ролей та alt-текстів для файлів):
    ```json
    [
      {
        "key": "new_pic",
        "is_primary": false,
        "sort_order": 10,
        "alt_text_uk": "Додаткове фото таблеток",
        "alt_text_en": "Additional photo of tablets"
      }
    ]
    ```
*   Поле форми **`new_pic`** (тип File): [обрати файл]

#### ✏️ Оновлення метаданих конкретної фотографії (`PATCH /api/admin/products/{id}/images/{img_id}`)
Надсилайте стандартний `application/json`. Дозволяє оновити прив'язку до варіації, сортування, роль або локалізовані alt-тексти:

**Тіло запиту (JSON):**
```json
{
  "sort_order": 2,
  "alt_text_uk": "Новий альт-текст українською",
  "alt_text_en": "New alt text in English"
}
```

---

## 🌐 3. Отримання даних у публічному API

Публічне API автоматично підставляє переклади мета-тегів та alt-текстів відповідно до мовного префіксу в URL (`/api/uk/...` або `/api/en/...`).

### А. Отримання товару (`GET /api/{lang}/products/by-slug/{slug}`)
Якщо запит виконано на `/api/uk/products/by-slug/finish-classic`, відповідь містить:
```json
{
  "id": "uuid-товару",
  "slug": "finish-classic",
  "name": "Таблетки для посудомийки Classic",
  "meta_title": "Таблетки для посудомийних машин Classic купити",
  "meta_description": "Екологічні таблетки для посудомийки. Ефективно видаляють жир.",
  "meta_keywords": "таблетки, посудомийка, еко",
  "images": [
    {
      "id": "uuid-фото",
      "image_url": "https://...",
      "is_primary": true,
      "alt_text": "Таблетки для посудомийки Classic 60 шт."
    }
  ]
}
```
Якщо зробити запит на `/api/en/products/by-slug/dishwasher-tablets-classic`, поля автоматично перемикаються на англійські значення:
```json
{
  "id": "uuid-товару",
  "slug": "dishwasher-tablets-classic",
  "name": "Dishwasher Tablets Classic",
  "meta_title": "Buy Dishwasher Tablets Classic Online",
  "meta_description": "Eco dishwasher tablets. Removes grease effectively.",
  "meta_keywords": "tablets, dishwasher, eco",
  "images": [
    {
      "id": "uuid-фото",
      "image_url": "https://...",
      "is_primary": true,
      "alt_text": "Dishwasher Tablets Classic 60 pcs pack"
    }
  ]
}
```

### Б. Отримання категорій (`GET /api/{lang}/categories`)
Повертає плоский список категорій із мовними мета-тегами:
```json
[
  {
    "id": "uuid-категорії",
    "slug": "dlia-posudu",
    "name": "Для посуду",
    "meta_title": "Купити засоби для миття посуду | AquaWheel",
    "meta_description": "Найкращі екологічні засоби для миття посуду та посудомийних машин."
  }
]
```
