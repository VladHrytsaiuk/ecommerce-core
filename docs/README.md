# Документація AquaWheel Backend

Ласкаво просимо до технічної документації бекенд-частини магазину AquaWheel. Ми розбили документацію на логічні розділи для зручнішого пошуку.

## 📂 Структура документації

### 🏗️ Архітектура (`architecture/`)
- [**Modules**](architecture/modules.md) — Опис усіх доменних модулів системи (Order, Product, Auth тощо).
- [**Infrastructure**](architecture/infrastructure.md) — Огляд інфраструктури, деплою та сервісів.
- [**Database**](architecture/database.md) — Структура бази даних, ключові таблиці.
- [**Search Architecture**](architecture/search_architecture.md) — Як працює пошук та фільтрація в каталозі.

### 📚 Гайди (`guides/`)
- [**Setup**](guides/setup.md) — Інструкція з локального розгортання проекту.
- [**Deployment Guide**](guides/deployment.md) — Як деплоїти бекенд на Production сервер (Docker, Nginx, SSL).
- [**Development Guide**](guides/development_guide.md) — Базові правила розробки, написання коду та структурування.
- [**Adding a New Module**](guides/new_module.md) — Покроковий туторіал зі створення нового модуля по Clean Architecture.
- [**Docker Guide**](guides/docker_guide.md) — Робота з Docker та контейнерами.
- [**Database Migrate**](guides/database_migrate.md) — Як створювати та запускати міграції.
- [**Git Flow**](guides/git_flow.md) — Правила роботи з гілками та комітами.
- [**Testing**](guides/testing.md) — Як писати та запускати тести.
- [**Logger**](guides/logger.md) — Робота з логуванням у системі.

### 🚀 Фічі (`features/`)
- [**Admin Product CRUD**](features/admin_product_crud.md) — API керування товарами, варіаціями та атрибутами.
- [**Order Workflow (v2)**](features/order_workflow_v2.md) — Життєвий цикл замовлення та оплати.
- [**Integrations Overview**](features/integrations_overview.md) — Огляд інтеграцій (Нова Пошта, Укрпошта, LiqPay).
- [**Product Badges**](features/PRODUCT_BADGES.md) — Логіка бейджів (Новинка, Знижка, Хіт продажів).
- [**Phone Verification (SMS)**](features/phone_verification_obm_sms.md) — Інтеграція з Vodafone OBM для відправки OTP кодів.
- [**SEO Metadata Guide**](features/seo_metadata_guide.md) — Робота з мета-тегами та хлібними крихтами.
- [**TTN Workflow**](features/ttn_workflow.md) — Генерація та відстеження ТТН для доставки.

### 🔌 API (`api/`)
- [**Swagger Files**](api/) — Згенеровані файли Swagger. Ви можете переглянути документацію API за адресою `/swagger/index.html` після запуску сервера.

### 🗄️ Архів (`archive/`)
- Історичні документи, старі версії архітектури та gap-аналізи (`admin_api_gap_analysis.md`, `api_analysis.md`, `order_workflow.md`).

## 🚀 Швидкий старт для розробника

1. Налаштуйте своє локальне середовище: прочитайте [Setup Guide](guides/setup.md) та налаштуйте `.env`.
2. Ознайомтеся з [Development Guide](guides/development_guide.md) перед написанням коду.
3. Ознайомтеся з [Modules](architecture/modules.md), щоб зрозуміти з яких частин складається проект.
4. Документація ендпоінтів доступна у Swagger: `/swagger/index.html`.
