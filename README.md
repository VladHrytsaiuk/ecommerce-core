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
  Stripe, Redsys, Nova Poshta, Correos, and future adapters.
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
go mod tidy
go run ./cmd/migrate up
go run ./cmd/api
```

Update `DB_URL` and other required values in `.env` before running migrations.
Never commit `.env`; use `.env.example` for safe placeholders.

### Docker

```bash
cp .env.example .env
docker compose up --build
```

See the Docker Compose configuration and the migration command for the
environment-specific service names and database settings.

## Development

```bash
go test ./internal/...
go vet ./...
```

Some repository tests use Docker/Testcontainers. Run them in an environment
where Docker is available.

To verify the clean-slate database path (core migrations, enabled Inventory
module, and three-locale Catalog persistence):

```bash
go test -tags=integration ./cmd/migrate
```

### Legacy reference

Unported monolith packages are excluded from the default build with a `legacy`
build constraint. They are retained only as migration reference; use the
`legacy-monolith-baseline` Git tag when the complete predecessor behaviour must
be inspected or run.

Before changing module boundaries, providers, migrations, or store
configuration, read [ARCHITECTURE.md](ARCHITECTURE.md) and follow
[ROADMAP.md](ROADMAP.md).
