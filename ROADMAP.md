# Roadmap: building ecommerce-core from a clean slate

## Operating model

`ecommerce-core` is a new project. There is no production database, API, data
set, or store deployment to migrate. Existing monolith code is reference
material only; it is replaced module by module with target-state code.

Work in small commits. A clean baseline migration may be edited only until it
is applied outside local development; afterwards every schema change is a new
forward migration. Keep provider I/O outside database transactions and test
each module independently of Gin and a real provider.

## Phase 1 — Bootstrap and typed configuration

**Status: complete.**

- `internal/app/bootstrap.go` owns dependency construction.
- `internal/http/router.go` only attaches ready handlers and middleware.
- Store configuration validates before database connection.
- `internal/app/workers.go` owns worker lifecycle and graceful shutdown.

## Phase 2 — Core schema and localization foundation

**Status: complete.**

**Goal:** create the ideal PostgreSQL schema for a new engine, with normalized
translations from day one.

1. Keep `migrations/core/000001_init_core.*` as the new-database baseline:
   locales, users, base catalog, carts, orders, payments and deliveries.
2. Store multilingual content only in tables such as
   `category_translations` and `product_translations`, with `locale VARCHAR(10)`
   and unique `(locale, slug)` constraints. Do not add JSONB translations.
3. Core owns universal schema. Optional capabilities own migrations under
   `migrations/modules/<module>/`; apply core before enabled modules.
4. Rebuild GORM models and repositories against plural core table names and
   normalized translations. Category is the first reference implementation.
5. Add translation-list DTOs to new API contracts:
   `{"translations":[{"locale":"es","name":"...","slug":"..."}]}`.
6. Introduce locale policy from typed configuration; adding a locale must not
   require a code change.

**Definition of Done: complete.** A fresh PostgreSQL database migrates from
zero in the Testcontainers/CI smoke test; Category and Product read and write
three locales through normalized tables; `scripts/check-clean-architecture.sh`
guards active migrations and Catalog against JSONB/legacy localized maps; and
every retained legacy behaviour is tracked in `LEGACY_FEATURE_MAP.md` before
its source package is retired. The default repository build and test suite
compile without unported legacy packages.

## Phase 3 — Checkout, money, tax and provider ports

**Status: complete.**

**Goal:** build provider-neutral commerce workflows before any real adapter.

1. Add `internal/core/money`, `internal/core/tax`, and `internal/core/orderworkflow`.
2. Add `PaymentGateway` and `Carrier` ports in `internal/payments/domain` and
   `internal/delivery/domain`.
3. Implement `internal/checkout/application` with injected checkout and tax
   policies. Use stable string order-status codes.
4. Test workflows with fake gateways and carriers.

**Definition of Done: complete.** Checkout derives immutable item snapshots
from Catalog, applies the configured tax and checkout policies, then reserves
stock. `internal/core/orderworkflow` atomically creates the pending order and
associates reservations; payment success commits stock once, while gateway
failure/cancellation releases it and cancels the order. PaymentGateway and
Carrier are provider-neutral ports with fake-backed workflow/registry tests.
The cross-context PostgreSQL transaction adapter lives in
`internal/platform/postgres/orderworkflow`, so core does not depend on a
concrete Orders or Inventory repository.
Concrete SDK adapters and webhook transport are deliberately Phase 4.

## Phase 4 — Payment and delivery adapters

**Status: in progress — LiqPay is the first clean payment adapter, selected
through Bootstrap, with an idempotent webhook workflow and clean checkout
endpoint. Stripe is a clean Payment Intent adapter with verified, idempotent
webhooks; Redsys is a signed redirect adapter with a verified callback and
refund request. Nova Poshta is the first clean Carrier adapter with `httptest`
contract coverage. Payment success now persists a provider-neutral delivery
snapshot and one durable delivery job atomically; the dispatch worker claims,
retries and completes jobs outside transactions. Correos and additional payment
providers remain.**

**Goal:** select real integrations only in Bootstrap configuration.

1. Add adapters under `internal/adapters/payment/{liqpay,stripe,redsys}` and
   `internal/adapters/delivery/{novaposhta,correos}`.
2. Register only enabled adapters and their webhook routes.
3. Make callbacks idempotent and translate payloads to domain events.
4. Add contract tests using fixtures and `httptest`.

For delivery, extend the neutral `Carrier` request only with universal facts:
recipient, opaque selected service-point identifiers, item weight and declared
value. Never add a carrier-named field to Checkout, Orders or Core schema.

## Phase 5 — Inventory and Sync

**Goal:** support internal inventory and external ERP read-model mode.

1. Add module-owned `inventory` migrations and transactional reservations.
2. Add `sync` module migrations: outbox, external entity state and cursors.
3. In `external_1c` mode, block stock edits in the application layer and use
   authenticated, idempotent import/export ports.

## Global rules

- Do not fork for a store.
- Do not hardcode provider, locale, currency, tax or status rules in core.
- Do not store translatable business content in JSONB.
- Do not add vertical fields to core tables; create a module-owned table.
- Do not call a payment, carrier or ERP from inside a DB transaction.
- Update this roadmap, `ARCHITECTURE.md`, `.env.example` and API docs whenever
  a supported module or configuration capability changes.
