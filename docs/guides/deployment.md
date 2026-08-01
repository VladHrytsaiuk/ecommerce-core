# Deployment Guide (Деплой на Production)

Цей документ описує процес розгортання бекенду AquaWheel Store на Production сервері. 

Оскільки проєкт використовує базу даних Supabase (яка хоститься окремо), деплой бекенду зводиться до запуску серверної частини та налаштування зворотного проксі (Nginx/Caddy) для HTTPS.

---

## 1. Підготовка оточення (`.env`)

На сервері створіть файл `.env`. Він має відрізнятися від локального наступними важливими параметрами:

```bash
# Вказує додатку, що він працює в продакшені (вимикає дебаг-логи, ховає деякі внутрішні помилки)
APP_ENV=production

# Порт, на якому буде працювати сервер локально
PORT=8080

# Production URL бази даних Supabase (з Transaction pooling, порт 6543)
DB_URL="postgresql://postgres.[project]:[password]@aws-0-eu-central-1.pooler.supabase.com:6543/postgres?sslmode=disable"

# Домен вашого API (потрібно для генерації Swagger, лінків у email тощо)
API_HOST="api.aquawheel.com"

# Встановіть надійний і довгий секрет!
JWT_SECRET="дуже_складний_продакшен_секрет_який_ніхто_не_знає"

# --- Інтеграції (Бойові ключі) ---
# Нова Пошта
NOVA_POSHTA_API_KEY="..."
# LiqPay (Увага: використовуйте бойові ключі, а не тестові)
LIQPAY_PUBLIC_KEY="..."
LIQPAY_PRIVATE_KEY="..."
LIQPAY_SERVER_URL="https://api.aquawheel.com/api/payment/webhook/liqpay"
# Vodafone
OBM_USERNAME="..."
OBM_PASSWORD="..."
```

---

## 2. Запуск через Docker (Рекомендовано)

Найкращий спосіб запускати застосунок — використовувати `docker-compose`.

1. Завантажте ваш код на сервер.
2. Переконайтеся, що файл `.env` присутній у корені.
3. Виконайте збірку та запуск у фоновому режимі:
   ```bash
   docker-compose up -d --build
   ```

Щоб переглянути логи сервера:
```bash
docker logs -f aquawheel-backend
```

---

## 3. Міграції Бази Даних

**Перед** тим як додаток почне повноцінно працювати з новими фічами, потрібно застосувати міграції БД.

```bash
# Якщо ви використовуєте golang-migrate CLI на сервері:
migrate -path database/migrations -database "$DB_URL" up
```
*Детальніше про міграції читайте у [database_migrate.md](database_migrate.md).*

---

## 4. Налаштування Reverse Proxy (HTTPS)

Бекенд (Go) слухає порт `8080` по HTTP. Щоб він був доступний з інтернету безпечно по HTTPS, необхідно налаштувати Reverse Proxy.

### Приклад конфігурації Nginx

Встановіть Nginx та Certbot (для безкоштовних SSL сертифікатів Let's Encrypt). 

Файл конфігурації `/etc/nginx/sites-available/api.aquawheel.com`:
```nginx
server {
    server_name api.aquawheel.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        
        # Передаємо реальні IP користувачів до Go-сервера
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_addrs;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Активуйте конфігурацію та отримайте сертифікат:
```bash
sudo ln -s /etc/nginx/sites-available/api.aquawheel.com /etc/nginx/sites-enabled/
sudo systemctl reload nginx
sudo certbot --nginx -d api.aquawheel.com
```

---

## 5. Graceful Shutdown (Безпечне завершення)

Код у `cmd/api/main.go` підтримує **Graceful Shutdown**. Це означає, що при перезапуску або зупинці сервера (наприклад, через `docker-compose stop`), сервер:
1. Перестане приймати нові запити.
2. Дасть 5 секунд на завершення поточних запитів (наприклад, завершення оплати або збереження замовлення).
3. Коректно закриє з'єднання з базою даних.
4. І тільки потім вимкнеться.

Тому ви можете спокійно оновлювати сервер без страху різко обірвати сесію клієнта.
