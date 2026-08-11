# ecommerce-core

`ecommerce-core` is a flexible, modular Go backend engine for online stores.
It provides a stable commerce foundation while allowing each store to select
its integrations, locales, policies, and enabled modules through configuration.

## Core Philosophy

**One codebase. No store-specific forks.**

A new store is configured rather than rewritten. Payment providers, carriers,
languages, currencies, tax rules, checkout behaviour, inventory mode, and
optional modules are selected through environment configuration and Dependency
Injection in the Composition Root (`internal/app/bootstrap.go`).

The core owns durable commerce invariants such as orders, reservations,
security, and provider-neutral workflows. Integrations are isolated adapters,
not business logic embedded in the order flow.

## Features

- **Pluggable payments and delivery** — provider-neutral ports for LiqPay,
  Stripe, Redsys, Nova Poshta, DHL Express, Correos, and future adapters.
  LiqPay, Stripe, Redsys, Nova Poshta and DHL Express are implemented clean
  adapters. Correos remains deferred until its deployment-specific contract is
  available.
- **Flexible inventory** — run autonomously with internal inventory, or use a
  Master-Slave storefront-cache model synchronized with 1C or another ERP.
- **Database-level i18n** — normalized translation tables make product,
  category, attribute, and content localization scalable from day one.
- **Modular schema evolution** — stable core tables with additive module-owned
  migrations for inventory, vertical-specific data, and integrations.
- **Configurable commerce rules** — money, tax, checkout, shipping, and module
  behaviour are explicit policies rather than store-specific hardcode.

## Architecture and Roadmap

The repository is being built as a clean-slate commerce engine. Older code may
be used temporarily as an implementation reference, but no legacy database or
store deployment is a compatibility target.

- [Target Architecture](ARCHITECTURE.md) — architectural principles, module
  boundaries, ports/adapters, database ownership, inventory, and Sync.
- [Migration Roadmap](ROADMAP.md) — practical, phased migration plan with
  concrete packages, compatibility gates, and definitions of done.

## Getting Started

### Prerequisites

- Go (version declared in `go.mod`)
- PostgreSQL, or Docker and Docker Compose

### Local setup

```bash
git clone <repository-url>
cd ecommerce-core
cp .env.example .env
# Set strong JWT_SECRET and POSTGRES_PASSWORD in .env first.
go mod tidy
go run ./cmd/migrate up
go run ./cmd/api
```

Update `DB_URL` and other required values in `.env` before running migrations.
Never commit `.env`; use `.env.example` for safe placeholders.

After the Inventory migration, create an active warehouse and put its UUID in
`DEFAULT_WAREHOUSE_ID`. This selects the stock-reservation warehouse for the
initial checkout policy; it is separate from a carrier's sender address.

Create the first store owner after migrations, before using the protected
admin catalog endpoints. In the Docker starter use the CLI container, which
has access to the internal PostgreSQL hostname:

```bash
docker compose run --rm \
  -e OWNER_EMAIL=owner@example.com \
  -e OWNER_PASSWORD='use-a-long-unique-password' \
  cli create-owner
```

For a locally managed PostgreSQL instance, the equivalent is
`go run ./cmd/cli create-owner -email ... -password ...`.

Then obtain a JWT through `POST /api/auth/login` and use it as
`Authorization: Bearer <access_token>` for `/api/admin/...` routes.

### API v1 contract

The additive `/api/v1` surface is the stable client-integration contract. It
includes localized Catalog and Checkout routes, authenticated customer Orders,
and permission-protected Admin Facades. Catalog lists paginate in PostgreSQL;
they never materialize a full catalog in the API process. Successful responses
always contain `data` and `request_id`; collection responses additionally
contain `meta` with `page`, `limit`, `total`, `total_pages`, `has_next`, and
`has_previous`. Errors use `application/problem+json` with a stable `code`,
such as `INVALID_PAYLOAD` or `RESOURCE_NOT_FOUND`. Legacy `/api/*` routes
remain unchanged while clients migrate. The generated OpenAPI contract is
served at `/swagger/index.html` and stored in `docs/api/swagger.yaml`.
All v1 request bodies are capped at 1 MiB (`PAYLOAD_TOO_LARGE` on overflow),
and v1 public traffic uses the Redis-backed distributed limiter when Redis is
enabled; its in-memory implementation is only the explicit local fallback.

The PostgreSQL client uses a bounded production pool (25 open / 10 idle
connections, 30-minute maximum lifetime and 5-minute maximum idle time).

### Optional product search projection

Set `ENABLED_MODULES=...,search` to enable the asynchronous Meilisearch
projection. It is not a transactional source of truth: Catalog writes a
minimal product-change event to PostgreSQL Outbox in the same transaction as
the product mutation, then the Search worker fetches a current Catalog
snapshot and indexes it. Configure `SEARCH_URL`, `SEARCH_MASTER_KEY` and
`SEARCH_INDEX_PREFIX`; the application fails before listening if an enabled
Search module cannot authenticate or configure its index. Locally, start it
with `docker compose --profile search up` and keep its master key only in
deployment secrets or `.env`, never in a client application.

Storefront discovery is available only while that module is enabled:
`GET /api/v1/catalog/{lang}/search` supports `q`, `page`, `limit`, `sort`,
repeated/comma-separated `brand`, `price_min`, `price_max`, `in_stock`, and
`attributes[{key}]` filters; its standard pagination metadata includes facet
counts. `GET /api/v1/catalog/{lang}/suggestions?q=...` supplies autocomplete.
Use `go run ./cmd/cli search-reindex --batch-size 200` after creating a new
index or intentionally rebuilding the disposable projection. It reads only
active Catalog products in bounded UUID-keyset batches (never `OFFSET`) and
can be safely interrupted. Search accepts at most 256 query characters, 12
attribute facet keys, 50 values per facet, and 100 facet values in total.
Every Meilisearch indexing task has a 30-second lifecycle deadline, so an
unhealthy provider cannot permanently occupy an Outbox worker.

### Optional media assets

Add `media,admin` to `ENABLED_MODULES` to enable secure media uploads. The
admin endpoint `POST /api/v1/admin/media/upload` requires `media:write`, a UUID
`Idempotency-Key`, and one `file` multipart part. Only magic-byte-validated
JPEG, PNG and WebP files up to 15 MiB are accepted; the original filename is
never used as an object key. Configure `MEDIA_PROVIDER` as `s3`, `minio`,
`r2`, or `cloudinary`. S3-compatible providers require `MEDIA_S3_BUCKET`,
`MEDIA_S3_REGION`, credentials and a public base URL; `minio` and `r2` also
require `MEDIA_S3_ENDPOINT`. Cloudinary requires `MEDIA_CLOUDINARY_URL`.
Start local MinIO with `docker compose --profile media up`; production may use
AWS S3, Cloudflare R2, or Cloudinary. Uploaded assets begin in
`quarantine`; a durable Outbox worker rejects images above 8192px per side or
16,000,000 pixels before processing and creates WebP variants after commit.
The media identity needs `s3:ListBucket` and object delete permission: a daily
reconciler removes only unreferenced or failed quarantine objects older than
24 hours.

### Customer identity and optional profiles

Password registration and login are always available at `POST /api/auth/register`
and `POST /api/auth/login`. Registration accepts an email or phone number and
a password; login accepts `login` (email or phone) and `password` (`email` is
kept as a compatibility alias). Both return a short-lived JWT containing the
string role.

Google OAuth is opt-in: set all of `GOOGLE_CLIENT_ID`,
`GOOGLE_CLIENT_SECRET`, and `GOOGLE_REDIRECT_URI`; otherwise no Google SDK
adapter is constructed. The public flow is `GET /api/auth/oauth/google/login`
and the configured callback is `GET /api/auth/oauth/google/callback`.

Store-specific customer data is opt-in as well. Add `user_profiles` to
`ENABLED_MODULES` and define `PROFILE_POLICY_JSON`; then an authenticated
customer can read and patch their policy-approved JSON document through
`GET` and `PATCH /api/me/profile`. The profile module owns its separate
`user_profiles` table, so per-store fields never alter the Core `users` table.
Concurrent profile edits use optimistic locking: the client receives `409
Conflict`, reloads the profile and retries its patch.

### Optional wishlist

Add `wishlist` to `ENABLED_MODULES` to enable `GET`, `POST`, and `DELETE`
at `/api/wishlist`. Each item refers only to a `product_variant_id`; the
module owns its `wishlist_items` table. Guests use the browser's existing
opaque `cart_session` cookie, while authenticated users use their JWT
identity. On successful registration, password login, or OAuth callback, the
optional module atomically merges that guest list into the user's list and
removes the source entries. With `wishlist` disabled, neither its routes nor
its dependencies are registered.

### Optional comparison

Add `comparison` to `ENABLED_MODULES` to enable `GET`, `POST`, and `DELETE`
at `/api/comparison`. The module uses the same JWT-or-opaque-`cart_session`
ownership model as Wishlist, and merges guest entries after authentication.
Comparison groups variants by their Catalog category, so only comparable items
share one group; `COMPARISON_MAX_ITEMS` is a typed, fail-fast validated limit
per category group (default `5`). During login merge, the newest unique items
across the guest and user group are retained deterministically.

### Optional reviews and ratings

Add `reviews` to `ENABLED_MODULES` to enable moderated product reviews. A JWT
customer creates one pending review per product with `POST /api/reviews/{id}`;
only approved reviews appear through `GET /api/reviews/{id}`. Admins approve,
reject, or delete via `/api/admin/reviews/{id}/status` and
`DELETE /api/admin/reviews/{id}`. The module owns its rating projection, while
Catalog reads it through a port and exposes `rating` on product responses when
the module is enabled.

### Optional SEO and product badges

Add `seo` to `ENABLED_MODULES` to manage localized product, category, and
future static-page metadata through `/api/admin/seo`. SEO is stored in the
module-owned polymorphic `seo_metadata` table; no Core Catalog columns change.

Add `badges` to enable `/api/admin/badges`. Badge display names use normalized
translations and can be assigned to products. Catalog enriches both single
product and product-list responses through optional reader ports. A product
list uses one bulk SEO query and one bulk badge query for the complete page,
never one query per product.

### Optional promotions

Add `promos` to `ENABLED_MODULES` to apply an active cart promo code during
checkout. The discount is calculated before tax and its rule is snapshotted on
the order. A promo redemption is reserved atomically with the pending order;
the payment webhook commits it only after payment succeeds, or releases it
when payment fails or is cancelled.

The checkout deadline is `CHECKOUT_RESERVATION_TTL`: the background worker
expires an unpaid order atomically, releasing stock and any reserved promo.
When a promotion covers the complete payable amount, checkout uses the local
`free` payment flow and commits the order without calling a payment provider.
If a verified `paid` webhook arrives after a local cancellation, the callback
is acknowledged and a durable `payment_anomalies` record is opened for manual
reconciliation rather than silently losing the captured payment.

### Optional transactional notifications

Add `notifications` to `ENABLED_MODULES` to consume the durable
`orders.paid.v1` event. Checkout snapshots `customer_email` and the active
locale into `order_contact_details`, so an order receipt remains addressed to
the original buyer even after a profile change. Receipt job payloads are stored
only as AES-256-GCM ciphertext; set `NOTIFICATION_ENCRYPTION_KEY` to a
base64-encoded 32-byte key (`openssl rand -base64 32`). Templates are selected
by the contact locale and fall back to `DEFAULT_LOCALE`. Set
`NOTIFICATION_EMAIL_PROVIDER` to `mock`, `smtp`, or `ses`; SMTP uses the
existing `SMTP_*` configuration and requires `SMTP_TLS_MODE=starttls` (or
`implicit` for SMTPS/465), while SES is an explicit safe placeholder
pending an AWS SDK credentials adapter.

### Optional Redis cache and rate limiting

Redis is opt-in: set `REDIS_ENABLED=true` and provide `REDIS_URL`, for example
`redis://redis:6379/0` when using the optional Compose profile:

```bash
docker compose --profile redis up
```

Startup verifies Redis with `PING`. Redis accelerates Catalog category reads
and provides a distributed fixed-window limit of five password-login attempts
per minute per client IP. It never carries payment, inventory, or Outbox truth.
With `REDIS_ENABLED=false`, Catalog caching is a no-op and login protection uses
a safe per-process fallback for local development; production replicas should
enable Redis for a shared policy.

### Operations endpoint and telemetry

The public API remains on `PORT`. Operational endpoints are isolated on
`MANAGEMENT_ADDR` (default `127.0.0.1:9090`): `/livez` reports process liveness,
`/readyz` verifies PostgreSQL and optional Redis, and `/metrics` exposes
Prometheus metrics. Bind this listener to a private network or orchestration
sidecar only; it is not part of the public API surface.

Every HTTP request creates an OpenTelemetry span, preserves or generates an
`X-Request-ID`, and records bounded-cardinality RED metrics. To export traces,
set `OTEL_ENABLED=true` and `OTEL_EXPORTER_OTLP_ENDPOINT` to an OTLP/HTTP
collector address such as `otel-collector:4318`. Structured logs emitted with
`logger.WithContext(ctx)` automatically include `request_id`, `trace_id`, and
`span_id`.

### Local observability stack

The default Compose deployment keeps the management listener internal. Start
Prometheus, Grafana, and Tempo with:

```bash
OTEL_ENABLED=true docker compose --profile observability up --build
```

Prometheus is available at `http://127.0.0.1:9091`, Grafana at
`http://127.0.0.1:3001`, and Tempo at `http://127.0.0.1:3200`. Grafana
automatically provisions Prometheus and Tempo datasources. Set
`GRAFANA_ADMIN_PASSWORD` in `.env` before starting the profile; the Compose
fallback is intentionally local-development-only.

### Admin RBAC and audit trail

Add `admin` to `ENABLED_MODULES` to apply the Admin module migration and build
the data-driven authorization foundation. Roles are assigned permissions in
PostgreSQL; authorization checks require a permission contract such as
`promos:write`, never a JWT role name. Catalog and permitted order-cancellation
mutations also pass through Admin Facades and append `admin.action.v1` in their
local transaction. A dedicated Outbox consumer stores immutable records in
`audit_logs`. Audit old/new payloads redact password-, token-, and secret-like
fields before they reach either the Outbox or the JSONB audit trail. Run
`go run ./cmd/cli grant-superadmin -email existing@example.com` after applying
Admin migrations to grant the seeded `SuperAdmin` role to the first user.

### Checkout contract

`POST /api/:lang/checkout/payment` starts payment for the caller's active
Cart. The request contains only buyer, delivery and redirect details; item
lines, `customer_id`, and a warehouse identifier are deliberately not accepted
from the browser. The server reads the Cart, resolves the authenticated buyer
or anonymous cart session, and reserves stock at `DEFAULT_WAREHOUSE_ID`.
The client must send a high-entropy `Idempotency-Key` HTTP header for every
logical checkout attempt and reuse that key only when retrying the same request.
If the first response is lost after payment creation, the retry replays the
existing checkout and returns fresh provider session data without creating a
second order or reservation.

`POST /api/:lang/checkout/delivery-options` uses that same active Cart to
return provider-neutral delivery options. It sends server-side item weights and
the configured currency to the selected enabled carrier; it does not reserve
stock or create an order.

When delivery is selected, `POST /api/:lang/checkout/payment` must include the
chosen `delivery_option_code`. Checkout re-quotes that code server-side and
persists its amount in the immutable order snapshot; the payment amount is
`items + tax + shipping`.

### Docker

```bash
cp .env.example .env
docker compose up -d --build
```

The starter Compose stack creates PostgreSQL, runs the one-shot migration
container, then starts the API only after migration succeeds. It binds the API
to `127.0.0.1:8080`; put a TLS reverse proxy in front of it for production.

See the Docker Compose configuration and the migration command for the
environment-specific service names and database settings.

## Development

```bash
go test ./internal/...
go vet ./...
```

Some repository tests use Docker/Testcontainers. Run them in an environment
where Docker is available.

### LiqPay development configuration

The first clean payment adapter is LiqPay. To enable it locally, set
`PAYMENT_PROVIDERS=liqpay`, `PAYMENT_DEFAULT=liqpay`, its two keys, and
`LIQPAY_CALLBACK_URL` (for example
`https://api.example.com/api/webhooks/payments/liqpay`). The checkout endpoint
is `POST /api/:lang/checkout/payment`; verified callbacks use the generic route
`POST /api/webhooks/payments/liqpay`.

### Stripe development configuration

Set `PAYMENT_PROVIDERS=stripe`, `PAYMENT_DEFAULT=stripe`,
`STRIPE_SECRET_KEY`, and `STRIPE_WEBHOOK_SECRET`. Stripe creates a Payment
Intent and the checkout response returns its short-lived `client_secret`; the
storefront completes that flow using Stripe.js. Verified callbacks use
`POST /api/webhooks/payments/stripe`.

### Redsys development configuration

Set `PAYMENT_PROVIDERS=redsys`, `PAYMENT_DEFAULT=redsys`, the merchant FUC,
terminal, signing key, callback URL, and numeric ISO currency code (for
example, `978` for EUR). Checkout returns a provider-hosted redirect URL and
the signed `payment_form` fields that the storefront must POST unchanged.
Verified callbacks use `POST /api/webhooks/payments/redsys`.

### DHL Express development configuration

Set `SHIPPING_PROVIDERS=dhlexpress` and `SHIPPING_DEFAULT=dhlexpress`, then
provide the `DHL_EXPRESS_*` account, sender-address, product-code and standard
parcel-dimension values shown in `.env.example`. The MyDHL adapter uses DHL's
test URL by default; set the live base URL only with production credentials.
An active DHL Express customer account is required. This initial adapter
accepts domestic shipments only: international labels remain blocked until the
core has a customs-item module (commodity description, origin and HS data).

To verify the clean-slate database path, including the enabled Inventory module,
three-locale Catalog persistence, and atomic order/reservation workflow:

```bash
go test -tags=integration ./cmd/migrate ./internal/platform/postgres/orderworkflow
```

### Legacy reference

Unported monolith files are excluded from dependency resolution and the default
build. They are retained only as migration reference; use the
`legacy-monolith-baseline` Git tag when the complete predecessor behaviour must
be inspected or run.

Before changing module boundaries, providers, migrations, or store
configuration, read [ARCHITECTURE.md](ARCHITECTURE.md) and follow
[ROADMAP.md](ROADMAP.md).
