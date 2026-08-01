# 📘 Повне Технічне Керівництво: Products API для Frontend-команди

**Версія:** 1.1
**Автор:** Tech Lead
**Статус:** Регламент для розробки

Цей документ є **основним джерелом правди** для розробки клієнтської частини (публічний сайт та адмін-панель) у взаємодії з сутностями "Товар" (`Product`) та "Варіація" (`Variation`). Він базується на актуальному коді бекенду. Неухильне дотримання цих правил є обов'язковим.

---

## 1. Базові концепти архітектури (Products vs Variations)

У нашій системі ці сутності розділені, щоб забезпечити максимальну гнучкість.

### 1.1 Product (Батьківський Товар)
- **Що це?** Це абстрактна сутність, маркетингова "картка". Наприклад, "Пральний порошок Ariel".
- **Характеристики:** Містить загальний опис, бренд, категорію, загальні для всіх варіацій фото.
- **Не продається:** Ви не можете додати в кошик "просто Ariel". Ви додаєте конкретну упаковку.

### 1.2 Variation (Торгова Пропозиція / SKU)
- **Що це?** Це **конкретний, фізичний товар**, який лежить на складі та який можна купити. Наприклад, "Пральний порошок Ariel, 3 кг" або "Пральний порошок Ariel для кольорового, 1.5 кг".
- **Характеристики:** Має власну ціну, `SKU`, `barcode`, вагу (`weight`), кількість в упаковці (`quantity_value`), та власні, унікальні для неї фотографії.

**Ключова ідея:** Один `Product` може мати багато `Variations`.

### 1.3 Генерація `slug` (SEO-friendly URL)
Слаг генерується на бекенді автоматично для кожної варіації, щоб забезпечити унікальні URL. Логіка наступна:
1.  Береться `slug` батьківського товару (напр. `pralniy-poroshok-ariel`).
2.  До нього через дефіс додається суфікс, згенерований з **першого непустого** поля варіації у такому порядку:
    -   `SKU` (напр., `ariel-color-3kg`) -> `pralniy-poroshok-ariel-ariel-color-3kg`
    -   `quantity_value` + `unit` -> `pralniy-poroshok-ariel-3-kg`
- **Fallback:** Якщо всі поля порожні, генерується унікальний ідентифікатор.

---

## 2. Отримання товару та логіка URL (CRITICAL SECTION)

Це найважливіший розділ для коректної роботи сторінки товару.

### 2.1 Ендпоінт
| Метод | URL                                    | Опис                                        |
| :---- | :------------------------------------- | :------------------------------------------ |
| `GET` | `/api/{lang}/products/by-slug/:slug` | Отримує повний об'єкт товару за слагом. |

### 2.2 Логіка "спливання" активної варіації
Бекенд реалізує зручну логіку: якщо ви передаєте `slug` конкретної варіації, вона гарантовано буде **першим елементом (`index: 0`)** у масиві `variations` у відповіді. Решта варіацій сортується за зростанням `quantity_value`, а потім ціни.

**Приклад:** У товару є варіації "30 шт" та "60 шт".

**Запит 1:** `GET /api/uk/products/by-slug/finish-classic` (слаг батьківського товару)
**Відповідь 1 (JSON на основі `ProductResponse` DTO):**
```json
{
  "id": "prod-uuid-1",
  "slug": "finish-classic",
  "name": "Finish Classic",
  "variations": [
    { "id": "var-uuid-30", "slug": "finish-classic-30", "quantity_value": 30, "price": 25000, "sku": "FIN-CL-30" },
    { "id": "var-uuid-60", "slug": "finish-classic-60", "quantity_value": 60, "price": 45000, "sku": "FIN-CL-60" }
  ]
}
```

**Запит 2:** `GET /api/uk/products/by-slug/finish-classic-60` (слаг варіації "60 шт")
**Відповідь 2 (JSON):** Варіація "60 шт" тепер на першому місці!
```json
{
  "id": "prod-uuid-1",
  "slug": "finish-classic",
  "name": "Finish Classic",
  "variations": [
    { "id": "var-uuid-60", "slug": "finish-classic-60", "quantity_value": 60, "price": 45000, "sku": "FIN-CL-60" },
    { "id": "var-uuid-30", "slug": "finish-classic-30", "quantity_value": 30, "price": 25000, "sku": "FIN-CL-30" }
  ]
}
```

### 2.3 **ОБОВ'ЯЗКОВА** поведінка фронтенду
Коли користувач на сторінці товару обирає іншу варіацію (інший колір, розмір, кількість):

1.  **НЕ РОБИТИ НОВИЙ ЗАПИТ ДО API.** Всі дані про всі варіації вже завантажені.
2.  Знайти у себе в стані об'єкт вибраної варіації.
3.  **ОБОВ'ЯЗКОВО** оновити URL у браузері на `slug` цієї варіації.

**Навіщо це потрібно:**
-   **SEO:** Кожна варіація індексується пошуковими системами за унікальним URL.
-   **User Experience:** Користувач може скопіювати посилання та бути впевненим, що інша людина побачить той самий розмір/колір.

**Приклад коду (React/Next.js):**
```javascript
const ProductPage = ({ product }) => {
  const router = useRouter();
  const [activeVariation, setActiveVariation] = useState(product.variations[0]);

  const handleVariationChange = (newVariation) => {
    setActiveVariation(newVariation);
    // Оновлюємо URL без перезавантаження сторінки
    router.push(`/products/${newVariation.slug}`, undefined, { shallow: true });
  };
  // ...
};
```

---

## 3. Створення та редагування товарів (Admin Panel)

| Метод   | URL                     | Опис                                      |
| :------ | :---------------------- | :---------------------------------------- |
| `POST`  | `/api/admin/products`     | Створення нового товару.                |
| `PATCH` | `/api/admin/products/:id` | **Часткове** оновлення існуючого товару. |

### 3.1 Детальний `payload` для створення (`POST`)
Запит надсилається як `multipart/form-data`. Він містить поле `payload` (JSON-рядок) та поля з файлами.

**Headers:**
- `Content-Type: multipart/form-data`
- `Authorization: Bearer <your_jwt_token>`

**Body `payload` (на основі `CreateProductRequest` DTO):**
```json
{
  "brand_id": "d1b7a6c9-e8f0-4d5a-bcde-1234567890ab",
  "category_id": "7ec3a1d3-3bdf-4d5a-8b8a-1a8a2c69d8a1",
  "is_active": true,
  "name_uk": "Навушники Sony WH-1000XM5",
  "name_en": "Sony WH-1000XM5 Headphones",
  "description_uk": "Найкраще шумозаглушення на ринку. Технологія Dual Noise Sensor та процесор V1 розкривають повний потенціал шумозаглушення.",
  "usage_instructions_uk": "Увімкніть навушники, затиснувши кнопку живлення. Підключіть через Bluetooth до вашого пристрою.",
  "variations": [
    {
      "id": "temp-front-uuid-1",
      "sku": "SONY-XM5-BLK",
      "price": 1500000,
      "old_price": 1600000,
      "quantity_value": 1,
      "weight": 0.250,
      "is_active": true,
      "attribute_values": [
        { "attribute_id": 10, "value_string_uk": "Чорний", "value_string_en": "Black" }
      ],
      "badge_ids": [3]
    },
    {
      "id": "temp-front-uuid-2",
      "sku": "SONY-XM5-SLV",
      "price": 1500000,
      "quantity_value": 1,
      "weight": 0.250,
      "is_active": true,
      "attribute_values": [
        { "attribute_id": 10, "value_string_uk": "Сріблястий", "value_string_en": "Silver" }
      ]
    }
  ],
  "attribute_values": [
    { "attribute_id": 1, "value_string_uk": "Bluetooth 5.2" }
  ],
  "badge_ids": [1],
  "images_meta": []
}
```
**Обов'язкові поля:** `brand_id`, `category_id`, `is_active`, `name_uk`, `variations`.

---

## 4. Робота з фотографіями (МАКСИМАЛЬНО ДЕТАЛЬНО)

### 4.1 Завантаження: `multipart/form-data` та Cloudinary
Бекенд очікує формат `multipart/form-data` та автоматично завантажує файли у хмарне сховище **Cloudinary**.

| Метод | URL                               | Опис                                         |
|:------|:----------------------------------|:---------------------------------------------|
| `POST`| `/api/admin/products/:id/images`  | Довантаження нових фото до існуючого товару. |

**Логіка мапінгу файлів (`key`)**

`key` — це **унікальне ім'я (ключ)**, яке ви самі вигадуєте, щоб зв'язати файл зображення з його описом у JSON. Це "міст" між файлом та його метаданими.

**Приклад (JS `FormData`):**
```javascript
const formData = new FormData();

// Файли, які користувач вибрав в <input type="file">
const mainImageFile = ...; // File object for 'photo1.jpg'
const variationImageFile = ...; // File object for 'photo2.jpg'

// 1. Метадані фото (це JSON, який передається як РЯДОК)
const imagesMeta = [
    {
      "key": "main_product_image", // Вигадуємо ключ.
      "is_primary": true,
      "is_hover": false,
      "sort_order": 1,
      "variation_id": null // null - це фото для батьківського товару
    },
    {
      "key": "photo_for_black_variation", // Вигадуємо інший ключ.
      "is_primary": false,
      "sort_order": 2,
      "variation_id": "temp-front-uuid-1" // Прив'язка до варіації по ID
    }
];

// 2. Додаємо payload (в нашому випадку він називається 'payload', не 'images') та файли
// ВАЖЛИВО: JSON з метаданими йде у полі 'payload'.
// Файли йдуть під своїми унікальними ключами.
formData.append('payload', JSON.stringify(imagesMeta)); 
formData.append('main_product_image', mainImageFile); // Ключ файлу збігається з "key"
formData.append('photo_for_black_variation', variationImageFile);

// 3. Відправляємо
axios.post(`/api/admin/products/${productId}/images`, formData);
```

### 4.2 Оновлення порядку
| Метод   | URL                                      | Опис                                         |
|:--------|:-----------------------------------------|:---------------------------------------------|
| `PATCH` | `/api/admin/products/:id/images/reorder` | Масове оновлення порядку всіх фото товару. |

**Body (на основі `ReorderImagesRequest` DTO):**
```json
{
  "ids": [
    "uuid-img-3",
    "uuid-img-1",
    "uuid-img-2"
  ]
}
```
Бекенд оновить поле `sort_order` для всіх фотографій товару відповідно до порядку у цьому масиві.

### 4.3 Видалення фото
| Метод    | URL                                        | Опис                      |
|:---------|:-------------------------------------------|:--------------------------|
| `DELETE` | `/api/admin/products/:id/images/:img_id` | Видалення одного фото. |

При успішному запиті бекенд видаляє запис з БД та асинхронно посилає запит на видалення файлу з Cloudinary.

Також можна видалити фото при оновленні товару, передавши поле `images_to_delete` в `payload` для `PATCH /api/admin/products/:id`.

---

## 5. Набори товарів (Bundles / Sets)

Набір — це віртуальний товар, що складається з кількох інших варіацій.

### 5.1 Створення/Оновлення набору
У `POST /api/admin/products` або `PATCH /api/admin/products/:id` передаються наступні поля:
- `is_bundle: true`
- `price_strategy`: 'dynamic' або 'manual'.
- `bundle_items`: Масив, що описує склад (на основі `BundleItemInput` DTO).

**Приклад `payload` для створення набору:**
```json
{
  "brand_id": "...",
  "category_id": "...",
  "is_active": true,
  "name_uk": "Супер набір для чистоти",
  "is_bundle": true,
  "price_strategy": "dynamic",
  "bundle_items": [
    { "variation_id": "uuid-варіації-шампуня", "quantity": 1 },
    { "variation_id": "uuid-варіації-кондиціонера", "quantity": 1 }
  ],
  "variations": [
    {
      "sku": "SET-CLEAN-01",
      "price": 0,
      "is_active": true
    }
  ]
}
```

### 5.2 Приклад відповіді для рендеру набору
`GET /api/uk/products/by-slug/super-shampoo-set`
```json
{
  "id": "bundle-uuid-1",
  "name": "Набір 'Чисте Волосся'",
  "is_bundle": true,
  "price_strategy": "dynamic",
  "variations": [ 
    { 
      "id": "var-bundle-uuid", 
      "price": 90000, 
      "old_price": 100000, 
      "sku": "SET-01"
    }
  ],
  "bundle_items": [
    {
      "variation_id": "var-uuid-shampoo",
      "quantity": 1,
      "product_name": "Шампунь Herbal",
      "product_slug": "shampoo-herbal",
      "price": 50000,
      "image_url": "..."
    },
    {
      "variation_id": "var-uuid-conditioner",
      "quantity": 1,
      "product_name": "Кондиціонер Herbal",
      "product_slug": "conditioner-herbal",
      "price": 50000,
      "image_url": "..."
    }
  ]
}
```

---

## 6. Типові помилки фронтендера (та як їх уникнути)

1.  **Помилка:** "При виборі іншої варіації на сторінці товару нічого не відбувається або відбувається з перезавантаженням."
    -   **Причина:** Ви або робите новий API запит, або використовуєте `<a>` замість `router.push(..., { shallow: true })`.
    -   **Рішення:** Завжди оновлюйте URL через `history.pushState` та змінюйте активну варіацію локально в стані.

2.  **Помилка:** "На сторінці товару завжди відображається перша варіація, навіть якщо я перейшов за посиланням на іншу."
    -   **Причина:** Ви ігноруєте логіку "спливання" і завжди рендерите `product.variations[0]`.
    -   **Рішення:** Ваша "активна" варіація при першому завантаженні — це завжди `product.variations[0]`, оскільки бекенд вже подбав про правильне сортування.

3.  **Помилка:** "Фотографії завантажуються, але не прив'язуються до правильних варіацій."
    -   **Причина:** `key` у JSON-`payload` не збігається з іменем поля, під яким ви відправляєте файл у `FormData`.
    -   **Рішення:** Ретельно перевірте відповідність ключів. Використовуйте `console.log(Array.from(formData.entries()))` перед відправкою для дебагу.
