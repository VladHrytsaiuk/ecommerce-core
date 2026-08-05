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
  Stripe, Redsys, Nova Poshta, Correos, and future adapters. LiqPay, Stripe,
  Redsys, and Nova Poshta are implemented clean adapters. Correos remains
  deferred until its deployment-specific contract is available.
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
