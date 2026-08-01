# Roadmap: Strangler Fig migration to ecommerce-core Target Architecture

## Purpose and operating model

This roadmap migrates the current Gin monolith to the target architecture in
[`ARCHITECTURE.md`](ARCHITECTURE.md) without a big-bang rewrite. At every step,
the application must compile, existing API routes must keep their behaviour,
and the old implementation remains active until its replacement is covered by
tests and switched on deliberately.

The pattern is:

```text
existing route/service -> compatibility facade -> new port/application service
                                          \-> legacy implementation (temporary)
```

For a solo developer, complete one small, reviewable vertical slice at a time.
Do not run multiple partially migrated workflows in parallel.

## Global guardrails

These rules apply to every phase.

- Work in small commits: one mechanical move, contract, adapter, migration, or
  behaviour change per commit. Do not combine formatting, renaming and logic
  changes.
- Before and after each step run `go test ./internal/...`; where Docker is
  available also run the repository/integration suites. In restricted CI,
  document skipped Testcontainers tests separately from code failures.
- Preserve existing HTTP endpoints and DTOs behind compatibility handlers until
  a versioned replacement is available. Additive endpoints/configuration are
  safer than breaking edits.
- Never move, edit, renumber, or split a migration that has already been
  applied to any shared environment. Migration history is immutable.
- Do not call an external payment, carrier, or ERP system in a database
  transaction.
- Each new outbound side effect must be idempotent and observable: correlation
  ID, provider/external ID, retry state and structured logs.
- Treat the current old code as a dependency, not as code to delete early.
  Delete a legacy path only after the new path is live, tested and unused.

## Phase 0 — Baseline and safety net

### Goal

Establish a reproducible baseline before changing package boundaries. This is a
short prerequisite phase, not an architectural rewrite.

### Steps

1. Record the existing public API routes and key flows: create cart, checkout,
   LiqPay callback, manager confirmation/TTN, shipment lookup, product CRUD.
   Use `docs/api/swagger.yaml` as the snapshot and add a small smoke-test list
   to `docs/`.
2. Add a compile-only CI command (`go test ./...` or a documented split where
   Testcontainers needs Docker) and make `go vet ./...` part of local checks.
3. Add characterization tests before moving behaviour:
   - payment URL generation and webhook status mapping in
     `internal/payment/service/payment_service_test.go`;
   - carrier creation/tracking behaviour in
     `internal/shipping/service/`;
   - order creation/status transitions in `internal/order/service/`;
   - locale fallback behaviour in `internal/http/middleware/locale.go`.
4. Audit `.env` locally, keep it ignored, rotate any credential that may have
   been exposed, and keep only placeholders in `.env.example`.
5. Resolve or explicitly park unrelated working-tree changes before beginning
   the migration. In particular, do not mix the currently reported deletions
   of `api`, `app`, and `main` with this roadmap.

### Definition of Done

- A new developer can run the documented checks and distinguish environment
  failures (Docker/listener permissions) from test failures.
- Existing checkout, payment callback and shipping flows have characterization
  tests or an explicit manual smoke checklist.
- The working tree contains only intentional changes for the next phase.

---

## Phase 1 — Bootstrap and typed configuration

### Goal

Make initialization explicit and configurable while preserving all current
implementations. `internal/http/router.go` becomes an HTTP composition layer;
`internal/app/bootstrap.go` becomes the sole Composition Root.

### Steps

1. Create `internal/app/bootstrap.go` with an `Application` struct containing
   already-built handlers, workers and lifecycle hooks. Initially it may call
   the same constructors currently called by `InitRouter`.
2. Move dependency construction from `internal/http/router.go` into
   `app.Bootstrap` incrementally:
   - first shared infrastructure (logger, DB repositories, token maker,
     email/SMS/storage);
   - then catalog/cart/wishlist/discount services;
   - then payment/order/shipment services and workers.
   Keep `InitRouter` temporarily, but change its signature to accept the
   constructed `Application` or a small `HTTPDependencies` struct.
3. Change `cmd/api/main.go` to call `app.Bootstrap(ctx, cfg, database,
   tokenMaker)` and then `http.NewRouter(app)`. Keep a thin temporary wrapper
   named `InitRouter` if that avoids a large route diff.
4. Split `internal/platform/config/config.go` into typed nested configuration
   while retaining a compatibility `Config` during the transition:

   ```text
   internal/app/config.go             # StoreConfig + Validate()
   internal/platform/config/config.go # environment loading only, temporary
   ```

   Add `Store`, `Locales`, `Money`, `Tax`, `Payments`, `Delivery`, `Inventory`,
   `Modules`, and `Checkout` sections. Do not yet change business behaviour.
5. Add `Validate()` in bootstrap. Initially validate only current legal values:
   `liqpay`, `novaposhta`, `UAH`, `uk/en`. Add new configuration fields with
   defaults matching production, so deployment is backwards compatible.
6. Move route registration out of constructor-heavy code. The existing calls to
   `paymentHttp.RegisterWebhookRoutes`, `shipmentHttp.RegisterShipmentRoutes`,
   `orderHttp.RegisterOrderRoutes`, and catalog route registration remain, but
   receive prebuilt services/handlers.
7. Move worker start/stop ownership out of `router.go` into
   `internal/app/workers.go`. Include the existing cleanup workers, sitemap
   worker, payment worker and tracking worker; preserve cancellation via the
   application context.

### What not to do yet

- Do not introduce Stripe, Correos, 1C, new migrations, or new HTTP routes.
- Do not change `payment_service.go` internals merely because its constructor
  moved.
- Do not pass the new global config into more services; reduce it later to
  narrow policy structs.

### Definition of Done

- `cmd/api/main.go` starts the application through `internal/app/bootstrap.go`.
- `internal/http/router.go` creates no DB repository, SDK client, provider,
  worker or business service; it only attaches prebuilt handlers/middleware.
- Startup fails before listening when configuration is invalid, and current
  `.env` values remain valid without manual changes.
- Existing routes, workers and tests compile and behave identically.

---

## Phase 2 — Migration ownership and localization foundation

### Goal

Create a safe forward path for module-owned schema and locale-driven content
without rewriting historical database state or breaking current `uk/en` APIs.

### Steps

1. Freeze the current flat `migrations/000001...000054` history. Do **not**
   rename or split applied files on a production database.
2. Update `cmd/migrate/main.go` into a migration orchestrator that can run
   ordered sources. First support a compatibility source for the existing flat
   directory, then add:

   ```text
   migrations/core/
   migrations/modules/inventory/
   migrations/modules/sync/
   migrations/modules/cosmetics/
   ```

   New migrations go only into the new layout. Document the deterministic order
   and module dependencies.
3. For a fresh install, create a tested baseline strategy: either a generated
   core baseline at the current schema version or a runner that applies the
   immutable legacy chain before new core/module migrations. Do not duplicate
   migration version numbers across independently tracked `golang-migrate`
   sources without separate migration state tables.
4. Add a forward-only core migration that introduces/normalizes `locale`:
   - create or migrate `language` to `locale(code VARCHAR(10), ...)`;
   - widen existing `language_code` columns to `VARCHAR(10)`;
   - create indexes and unique constraints such as `(locale, slug)`;
   - migrate values without dropping existing translations.
5. Keep and extend relational translation tables in the current domain models:
   - `internal/product/domain/product.go`: `ProductTranslation`;
   - `internal/category/domain/category.go`: category translations;
   - attributes, units, badges and future content pages.
   Rename Go/database fields from `LanguageCode` only through a backwards
   compatible mapping; the target name is `Locale`.
6. Stop adding `LocalizedMap` for new translatable data. Plan a separate,
   data-migration slice for every existing JSONB map (`Unit`, `Badge`, variation
   names, image alt text, attribute values). Convert one entity type at a time:
   create its `_translation` table, dual-read, backfill, dual-write, switch
   reads, then remove JSONB only in a later release.
7. Add `internal/core/locale` (or initially `internal/app/locale`) with a
   `LocalePolicy`. Refactor `internal/http/middleware/locale.go` to accept the
   supported/default/fallback locales from Bootstrap. Keep the `ua -> uk` alias
   only as temporary compatibility behaviour.
8. Introduce translation-list DTOs for new/versioned admin endpoints:

   ```json
   {"translations":[{"locale":"es","name":"...","slug":"..."}]}
   ```

   Keep legacy `name_uk`/`name_en` DTOs in
   `internal/category/delivery/http/dto.go` and product DTOs as adapters until
   clients migrate.

### Definition of Done

- Applied migration history is unchanged and a fresh database plus an existing
  database both migrate successfully through a tested path.
- New schema work has a clear owner under `core` or `modules/<module>`.
- Locale validation comes from configuration, not the hardcoded map in
  `middleware/locale.go`.
- New APIs can create/read three locales without code changes per language.
- No new multilingual business text is introduced as JSONB or `*_uk`/`*_en`.

---

## Phase 3 — Core workflow ports and application abstractions

### Goal

Separate checkout/order business decisions from LiqPay, Nova Poshta, UAH and
current carrier-specific order semantics. This phase creates ports and facades;
the legacy implementations still execute behind them.

### Steps

1. Create provider-neutral packages without moving SDK code yet:

   ```text
   internal/payments/domain/gateway.go
   internal/payments/application/service.go
   internal/delivery/domain/carrier.go
   internal/delivery/application/service.go
   internal/checkout/application/service.go
   internal/core/money/
   internal/core/tax/
   internal/core/orderworkflow/
   ```

2. Define `PaymentGateway` and `Carrier` ports as specified in
   `ARCHITECTURE.md`. Their input/output types use amounts, ISO currency,
   provider code, references and normalized statuses; they contain no LiqPay or
   Nova Poshta payload types.
3. Add a `Money` value type and `TaxPolicy`. Do not change the current database
   representation in one step: retain integer amounts, but pass a `Money`
   value through new use cases and snapshot currency/tax data on new orders via
   additive migrations.
4. Extract orchestration from `internal/order/service/order_service.go` into a
   Checkout application service. Start with a facade whose implementation calls
   the existing repositories, promo logic and shipment/payment compatibility
   interfaces. Preserve the current `CreateOrder` HTTP contract.
5. Replace direct `paymentDomain.PaymentService` and
   `shipmentDomain.ShipmentService` dependencies in `orderService` with narrow
   provider-neutral application interfaces. Keep adapters that delegate to the
   old services while the migration is incomplete.
6. Move hardcoded checkout rules into named policies: phone verification,
   guest checkout, minimum order, free shipping, tax calculation and payment
   required/not required. Initial policy values reproduce existing behaviour.
7. Replace numerical transition knowledge progressively. Add stable order-status
   codes alongside the current IDs in `internal/order/domain/order.go` and
   migrate `status_transitions.go`, payment completion, confirmation and
   tracking code to the code-based workflow. Keep ID mapping at the repository
   boundary until all flows are converted.
8. Add contract tests with fake `PaymentGateway` and fake `Carrier`; test
   checkout decisions independently of Gin, GORM, HTTP and provider payloads.

### Definition of Done

- Checkout/order application code imports neither `internal/integration/...`
  nor a payment/carrier SDK type.
- A fake gateway and fake carrier can exercise order creation, payment success,
  failure, fulfilment and cancellation in unit tests.
- Currency, tax and checkout rules are injected policies with defaults that
  preserve today’s store behaviour.
- Numeric status IDs are not used outside repository/legacy compatibility code
  for newly migrated workflows.
- Existing checkout API still creates orders and payment URLs successfully.

---

## Phase 4 — Provider adapters and switch-over

### Goal

Move LiqPay and Nova Poshta protocol code behind the Phase 3 ports. Bootstrap
selects only configured adapters; adding Stripe or Correos becomes an adapter
task, not an order refactor.

### Steps

1. Create adapter packages:

   ```text
   internal/adapters/payment/liqpay/
   internal/adapters/payment/stripe/       # interface/test skeleton only
   internal/adapters/payment/redsys/       # interface/test skeleton only
   internal/adapters/delivery/novaposhta/
   internal/adapters/delivery/correos/     # interface/test skeleton only
   ```

2. Move LiqPay signing, checkout payload generation, callback decoding and
   signature verification from `internal/payment/service/payment_service.go`
   into `adapters/payment/liqpay`. The adapter returns a normalized
   `PaymentEvent`; the payment application service performs idempotency,
   payment persistence, order transition and notifications.
3. Replace the fixed routes in
   `internal/payment/delivery/http/routes.go` and LiqPay request DTOs in the
   handler with a generic provider dispatcher:

   ```text
   POST /api/webhooks/payments/:provider
   ```

   Keep `/api/webhooks/liqpay` as a temporary compatibility route delegating to
   the dispatcher. Register routes only for configured gateways.
4. Move the Nova Poshta client from `internal/integration/novaposhta` into
   `internal/adapters/delivery/novaposhta` (or retain a low-level client there).
   Move the NP `switch`, TTN creation, tracking and status mapping from
   `internal/shipping/service/carrier_service.go` into the adapter. The carrier
   port receives/returns neutral shipment structures.
5. Change `internal/order/service/confirm_service.go` and
   `internal/order/service/tracking_worker.go` to work with the neutral carrier
   interface and shipment provider stored on the order. Remove assumptions that
   every shipment has a TTN or follows NP status codes.
6. Introduce provider factories in `internal/app/bootstrap.go`:
   `NewPaymentGateways(cfg.Payments)` and `NewCarriers(cfg.Delivery)`. Their
   registry maps configured codes to adapter instances. Validate credentials and
   reject unknown/disabled providers at startup.
7. Add adapter contract tests using `httptest` fixtures and application tests
   using fakes. Do not claim Stripe/Correos support until their adapter contract
   and webhook/quote/shipment tests are implemented.
8. After LiqPay/NP traffic uses ports in production and compatibility metrics
   show no callers of the old paths, delete:
   - `internal/payment/service/payment_service.go` LiqPay-specific logic;
   - `internal/shipping/service/carrier_service.go` NP-specific switch;
   - LiqPay-only DTOs/domain payloads and the old webhook route.
   Keep payment and shipment persistence repositories as provider-neutral code.

### Definition of Done

- `internal/payments` and `internal/delivery` application/domain packages have
  no imports from `internal/adapters` or provider SDKs.
- LiqPay and Nova Poshta run as adapters selected by Bootstrap configuration.
- A configured disabled provider has no active adapter and no registered
  webhook route.
- Webhook processing is idempotent and maps each provider event to the same
  order workflow semantics.
- The old LiqPay-only payment service and NP-only carrier switch have been
  removed only after their compatibility paths reach zero use.

---

## Phase 5 — Inventory, reservations and Sync/1C foundation

### Goal

Introduce inventory as a first-class module and make external ERP integration
reliable, while preserving autonomous internal mode as the default.

### Steps

1. Create the module boundary and migrations:

   ```text
   internal/inventory/domain/
   internal/inventory/application/
   internal/inventory/repository/postgres/
   internal/inventory/delivery/http/
   migrations/modules/inventory/
   ```

   Add `stock_item`, `stock_reservation` and, if needed, warehouse tables.
   Use a unique reservation key, expiry timestamp, status and variation ID.
2. Implement `InventoryService` operations: availability check, atomic reserve,
   release, commit, expiry cleanup and reconciliation. Use row locks or atomic
   conditional updates; write concurrency tests for simultaneous checkout of
   the last unit.
3. Integrate reservation into the new Checkout application service:
   reserve before creating a payment session; release on failure/expiry;
   commit after a successful normalized payment event; restore stock on a
   valid cancellation/refund policy.
4. Introduce `INVENTORY_MODE=internal|external_1c` in typed config and provide
   two implementations/policies:
   - `internal`: PostgreSQL inventory is authoritative; permitted admin stock
     mutations invoke `InventoryService`.
   - `external_1c`: local stock is a synchronized read model; inventory admin
     mutations return a domain-level read-only/source-of-truth error.
5. Create Sync boundaries and storage:

   ```text
   internal/sync/domain/
   internal/sync/application/
   internal/sync/delivery/http/
   internal/sync/worker/
   migrations/modules/sync/
   ```

   Add `integration_outbox`, `sync_cursor`/`external_entity`, and delivery
   attempt records. The exact naming is less important than explicit ownership
   and idempotency constraints.
6. Implement an outbox transaction with new-order creation. A worker publishes
   `OrderSnapshot` to an `OrderExporter`; retries with exponential backoff and
   idempotency key. Initial exporter may be a logging fake.
7. Add authenticated, versioned inbound endpoints such as
   `POST /api/integrations/:source/catalog-events`. The 1C adapter verifies the
   source request and maps it to `CatalogChange`/`StockChange`; Sync application
   services perform deduplication and update the local catalog/inventory model.
8. Add `internal/adapters/sync/one_c/` only after the above contracts exist.
   Implement a sandbox/fixture-backed 1C adapter, then enable it via bootstrap.
9. Add reconciliation and operational visibility: dead-letter state, retry
   counters, last successful cursor, stale-sync alert and an admin read-only
   status endpoint.

### Definition of Done

- Internal mode supports transactional reservation/commit/release without
  overselling under concurrent checkout tests.
- External mode blocks stock editing at the application boundary, while the
  storefront reads the synchronized local cache.
- Orders are persisted even if ERP is temporarily unavailable; export retries
  from the outbox and never duplicates a remote order.
- Inbound catalog/stock events are authenticated, versioned and idempotent.
- Inventory correctness and reservation lifecycle are owned by core workflow,
  not by a 1C, payment or delivery adapter.

---

## Final consolidation (after Phase 5)

Do not schedule this as a separate rewrite. Perform it only as each old path
reaches zero callers:

1. Remove compatibility wrappers and deprecated routes after a documented
   deprecation window.
2. Remove the hardcoded language map, UAH assumptions, AquaWheel branding in
   service code, numeric status workflow and remaining provider-specific
   imports from application packages.
3. Complete JSONB-to-`_translation` data migrations entity by entity, with a
   backfill and rollback plan.
4. Update `ARCHITECTURE.md`, this roadmap, Swagger and `.env.example` whenever
   a new port, module or configuration key becomes supported.

The migration is complete when a new store can select locales, money/tax policy,
payments, carriers, inventory mode and enabled modules through configuration;
no core code change is required to choose between already-supported options.
