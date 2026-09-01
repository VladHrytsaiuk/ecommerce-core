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

**Status: complete for LiqPay, Stripe, Redsys, Nova Poshta and DHL Express.
Correos is explicitly deferred until its customer-specific contract/API
documentation is available.**

**Goal:** select real integrations only in Bootstrap configuration.

1. Add adapters under `internal/adapters/payment/{liqpay,stripe,redsys}` and
   `internal/adapters/delivery/{novaposhta,dhlexpress,correos}`.
2. Register only enabled adapters and their webhook routes.
3. Make callbacks idempotent and translate payloads to domain events.
4. Add contract tests using fixtures and `httptest`.

For delivery, extend the neutral `Carrier` request only with universal facts:
recipient, opaque selected service-point identifiers, item weight and declared
value. Never add a carrier-named field to Checkout, Orders or Core schema.

**Definition of Done: complete (implemented adapters).** Bootstrap selects
only enabled gateways/carriers; clean Checkout creates Cart-derived order,
reservation, tax and shipping snapshots; gateway callbacks are verified and
idempotent; payment success creates one durable delivery job; Nova Poshta and
DHL Express adapters are covered by HTTP contract tests. `shipping_amount` is a forward-only
core snapshot and the gateway receives the exact immutable total. Checkout
requires an HTTP `Idempotency-Key`, from which it derives a stable checkout ID
so a browser retry cannot create a second order or payment attempt.

### Post-Phase-4 operational hardening

- Add a durable Refund application workflow and admin/API authorization model;
  existing gateway `Refund` methods are adapters only.
- Add carrier-side reconciliation by stable idempotency key before retrying an
  ambiguous shipment creation timeout, once every supported carrier exposes
  the necessary lookup semantics.
- Implement the Correos adapter after receiving its production contract,
  sender setup and service-point/location requirements.
- Add international DHL Express labels only together with a customs-item
  module; the existing adapter rejects them rather than sending incomplete
  declarations to the carrier.

## Phase 5 — Inventory and Sync

**Goal:** support internal inventory and external ERP read-model mode.

**Status: in progress.** The initial Sync module schema is additive and can be
enabled independently through `ENABLED_MODULES=...,sync`. It stores durable
outbox delivery state, external entity synchronization state and cursors.
`order.created` is written atomically with its Order; the outbox dispatcher
has an exclusive lease/retry lifecycle. The inbound StockChange application
workflow persists source/version/hash state and only accepts authoritative
absolute quantities in external inventory mode. A concrete authenticated ERP
transport, catalog identity/upsert policy and exporter adapter still require
the provider's contract and remain to be connected before `external_1c` can
be enabled.

1. Add module-owned `inventory` migrations and transactional reservations.
2. Add `sync` module migrations: outbox, external entity state and cursors.
3. In `external_1c` mode, block stock edits in the application layer and use
   authenticated, idempotent import/export ports.

## Phase 6 — Engagement modules

**Status: in progress.** Wishlist and Comparison are implemented optional
engagement modules.

1. Add a module-owned `wishlist_items` table with strict user/session owner
   exclusivity, foreign keys to Core users and Catalog variants, and partial
   unique indexes for idempotent add operations.
2. Enable its HTTP surface only through `ENABLED_MODULES=...,wishlist`; it
   serves both JWT users and the existing opaque anonymous cart session.
3. Subscribe the module to the narrow post-login application event assembled
   in Bootstrap, so guest items merge transactionally after authentication
   without Identity importing Wishlist.
4. Comparison owns category-scoped lists and applies a configured per-category
   item limit atomically; guest-category groups merge after login with an
   explicit newest-items retention policy.
5. Reviews is an opt-in module with authenticated pending submission, admin
   moderation, and a module-owned approved-rating projection exposed to
   Catalog through a port; it does not alter Core tables or import a Catalog
   repository.

## Phase 7 — Marketing and SEO

**Status: in progress.** SEO and Badges are implemented as optional modules.

1. SEO owns localized polymorphic metadata rather than columns on products or
   categories; Catalog reads it through a bulk reader port.
2. Badges own normalized translations and product associations; Catalog reads
   page data through a bulk port, avoiding N+1 queries.
3. Bootstrap wires module repositories only when `seo` or `badges` appears in
   `ENABLED_MODULES`; their admin routes are absent otherwise.
4. Promos is opt-in and decorates the Checkout price calculator. It snapshots
   the applied rule and participates in the atomic OrderWorkflow transaction:
   redemption is reserved before commit, committed after payment, and released
   on cancellation without admitting an over-limit concurrent checkout.
   Pending checkout expiry uses the same configured reservation deadline and
   atomically releases stock plus redemptions. Zero-total carts use a local
   free-order flow; late paid callbacks after cancellation create a durable
   manual-reconciliation anomaly instead of being retried indefinitely.

## Phase 8 — Notifications and background jobs

**Status: complete.** Core now provides a PostgreSQL transactional event
outbox. `orders.paid.v1` is appended atomically with the paid order and is
claimed through lease-based `FOR UPDATE SKIP LOCKED` delivery rows. The
optional Notifications module creates idempotent order-paid email jobs from
the event and records provider attempts. Recipient payloads use AES-256-GCM
encryption at rest, are decrypted only in worker memory, and templates resolve
by contact locale with `DEFAULT_LOCALE` fallback. Bootstrap selects `mock`,
SMTP, or the explicit SES integration boundary through configuration; provider
I/O remains outside the database transaction. The worker has panic recovery,
sanitized failure codes, a bounded retry policy and a durable `dead` delivery
state; notification jobs use fenced leases to prevent concurrent sends.

### Infrastructure foundation — Redis cache and distributed rate limiting

**Status: complete.** Redis is an opt-in capability selected with
`REDIS_ENABLED=true` and `REDIS_URL`. Bootstrap validates connectivity with a
bounded `PING` before listening, then supplies provider-neutral cache and rate
limiting ports. It is never used as a source of truth or as an Outbox broker.
Catalog category lookups are cached for ten minutes and invalidated after a
category write. Password login uses an atomic Redis fixed-window limit of five
attempts per minute per protected client-IP key. With Redis disabled, caching
is a no-op and the same login policy falls back to an explicitly per-process
limiter for constrained local environments.

## Phase 9 — Admin API, RBAC and Audit Logging

**Status: complete.** The opt-in `admin` module owns a normalized,
data-driven RBAC schema: roles, permissions, many-to-many assignments and an
active admin record with an `authorization_version`. Its Authorizer always
checks the active/version state in PostgreSQL, then caches the permission set
by versioned Redis key. Increasing the version makes stale grants unreachable
without depending on best-effort cache invalidation. `POST /api/admin/promos`
is the first dynamic-RBAC route and requires `promos:write`. Its Admin Facade
opens one local transaction for the promotion mutation and `admin.action.v1`
Outbox append. A dedicated `admin_audit` consumer persists idempotent audit
records by event ID, with recursive sanitization of password-, token-, and
secret-like fields before durable storage. Catalog and Orders facades remain
the next incremental migration. Catalog product/category creation and updates,
promotion creation, and safe pending-payment order cancellation use Admin
Facades; each writes its audit event in the mutation transaction. Legacy
role-name AdminMiddleware has been removed. The idempotent `SuperAdmin` seed
migration registers every current permission, and `cmd/cli grant-superadmin`
assigns the role to an existing user without an HTTP bootstrap endpoint.

## Phase 10 — Production infrastructure and observability

**Status: complete.** The application exposes a private management listener
with liveness, dependency readiness and Prometheus endpoints; the public Gin
API stays separate. OpenTelemetry context propagation and bounded RED metrics
are transport infrastructure, while structured Zap logs gain correlation IDs
only through `logger.WithContext(ctx)`. The production Docker image is a
cached multi-stage static Go build running as the distroless `nonroot` user;
the Compose `observability` profile provides Prometheus, Grafana and Tempo for
local use. GitHub Actions runs architecture checks, golangci-lint, race and
fresh-schema Testcontainers tests, then verifies the production Docker build
without publishing it.

## Phase 11 — Client integration and API polish

**Status: complete.** The additive `/api/v1` transport surface preserves
legacy routes unchanged while exposing Catalog, Checkout, customer Orders and
all implemented Admin Facades through a common success/pagination envelope and
RFC 9457 problem details with stable public error codes. Catalog pagination is
bounded by PostgreSQL `LIMIT`/`OFFSET` with a separate `COUNT(*)` metadata
query; it never loads a full catalog in the HTTP process. Strict credentialed
CORS and API-only security headers remain platform middleware and do not enter
business modules. Swagger annotations and generated active API artifacts cover
every v1 endpoint; CI regenerates them and rejects an uncommitted contract.
The public v1 boundary additionally enforces a 1 MiB streaming request-body
cap, distributed rate limiting when Redis is enabled, strict localized slug
validation and request-context cancellation through GORM into PostgreSQL.
The production PostgreSQL client also has a bounded pool and suppresses SQL
bind-value logging, protecting both database capacity and customer PII.

## Phase 12 — Search projection and discovery

**Status: complete.** The optional `search` module uses Meilisearch as a
non-authoritative product projection. Product create, update and delete Admin
Facade mutations append `catalog.product.changed.v1` in their existing SQL
transaction. A dedicated `search_indexer` Outbox consumer handles idempotent,
out-of-order delivery by reading a fresh Catalog snapshot for upserts and
deleting stale or unavailable documents. The Meilisearch adapter configures
searchable, filterable and sortable document fields on startup, while product
text, locale, category and future brand/attribute/price/availability facets
remain provider-neutral in `SearchDocument`. The public `/api/v1/catalog/:lang`
search and autocomplete endpoints validate bounded pagination and typed facet
filters, return RFC 9457 `SEARCH_UNAVAILABLE` when the projection is down, and
include Meilisearch facet distributions in standard pagination metadata. The
local `search-reindex` CLI restores the projection through UUID keyset batches
and the Core partial active-product index; it never scans an increasing
`OFFSET` prefix. Query and facet cardinality limits prevent oversized filter
expressions from reaching the Search provider. Every provider task has a
bounded deadline, so a stalled Meilisearch task follows Outbox retry/DLQ
handling instead of permanently occupying an indexer worker.

## Phase 13 — Media assets and object storage

**Status: complete.** The optional `media` module owns
asset metadata, variant records and processing attempts, while object bytes
remain in a provider-neutral object store. `POST /api/v1/admin/media/upload` is RBAC
protected, streams a single magic-byte-validated JPEG/PNG/WebP with a strict
15 MiB file limit, and uses only a server-generated quarantine object key.
The quarantine asset record and `media.asset.uploaded.v1` delivery are appended
in one PostgreSQL transaction. A pure-Go Outbox consumer rejects images above
8192px per dimension or 16,000,000 pixels before full decode, then creates
WebP thumbnail/product variants. Catalog stores asset links only after the
MediaReader confirms assets are ready. A daily, paginated object-store reconciliation
worker removes quarantine objects older than 24 hours that have no non-failed
asset record. The S3-compatible adapter is portable across AWS S3, MinIO and
Cloudflare R2; Cloudinary implements the same ObjectStore port.

## Phase 14 — Business reports and analytics

**Status: complete.** The optional `reports` module owns currency-separated
CQRS sales, product-sales and funnel read models for the permission-gated Admin
API. Paid and refunded order deliveries are idempotent through one durable
processed-event marker and one local transaction. A bounded asynchronous
rebuild takes a transaction-scoped PostgreSQL advisory lock shared with
projectors, deletes only its requested range, and reconstructs it from narrow
order and lifecycle snapshot ports. `reports:rebuild` is deliberately separate
from `reports:read`; health exposes active rebuild state and the last processed
event timestamp.

## Phase 15 — Financial and asynchronous-workflow hardening

**Status: complete.** All monetary values remain signed integer minor units.
Checkout allocates a cart-level promotion across immutable order lines with the
deterministic largest-remainder algorithm, so the invoice-level discount always
equals the sum of line discounts. The Outbox persists only bounded W3C trace
metadata and restores it for consumers; PostgreSQL and Redis operations produce
safe client spans without payloads, SQL, or credentials. Completed delivery
rows move to an archive in bounded `SKIP LOCKED` batches after a configurable
retention window, while high-churn delivery status changes use aggressive
per-table autovacuum settings. Dead deliveries and immutable domain events
remain queryable for manual recovery, Audit, and Reports.

## Phase 16 — Monobank acquiring

**Status: complete.** The Monobank adapter creates UAH invoices using integer
minor units and stores only the provider invoice identifier as the payment
reference. Its webhook adapter validates the raw request bytes with the
configured base64 PEM ECDSA public key before decoding JSON, then maps only
known invoice states to the provider-neutral payment event contract. The
generic webhook transport enforces a strict 1 MiB request-body cap without
truncating signed input. Immutable amount/currency matching remains inside the
existing locked OrderWorkflow transaction, so a verified provider callback
cannot change an order using a substituted amount.

## Phase 17 — Nova Poshta production logistics

**Status: complete.** Provider-neutral delivery selectors are wrapped by a
Redis-compatible cache decorator with 24-hour area/city and 6-hour
service-point TTLs; singleflight prevents a cold-cache stampede, while the
NoOp cache preserves direct provider reads. Nova Poshta shipment dispatch
reconciles the durable UUID sent in `InfoRegClientBarcodes` before every retry,
then retains a `dead` delivery job after five unsuccessful attempts for manual
handling. Tracking status codes map only inside the Nova Poshta adapter. The
tracker performs provider I/O before it opens the local transaction that locks
the delivery row, persists an idempotent delivery status and invokes the narrow
Orders state-machine bridge with `delivery_webhook` / `delivery_provider`.
Carrier refusal is modelled as a `delivery_refused` operational exception,
never as an unsafe automatic financial cancellation of a paid order.

## Phase 18 — Verified checkout and customer-profile policy

**Status: in progress (steps 1–2 complete).** Checkout now has deployment-level
guest, verified-email and verified-phone policies. It receives only the two
boolean verification facts it needs through a narrow `CustomerVerificationReader`
port implemented by Identity and assembled in Bootstrap; it never reads Identity
tables. Requests that require verification fail before inventory reservation or
payment-provider I/O. Identity stores the two durable verification flags on the
universal user aggregate, including a safe OAuth backfill for existing verified
provider identities.

The optional `customers` module owns typed self-service `customer_profiles`
and `customer_addresses` records. Its authenticated v1 API never accepts a
customer ID in a request body, so JWT ownership is enforced before every
profile or address mutation. A separate field-presence reader allows Checkout
to require typed fields or configured metadata keys without receiving profile
values. Orders remain independent: checkout copies its delivery data into the
existing immutable `order_delivery_details` snapshot, never an address-book ID.

## Phase 19 — Product options and multi-variant Catalog

**Status: in progress (steps 1–2 complete).** Catalog now owns normalized option
axes (`product_options`), their values and the `variant_option_values` matrix;
`product_variants` remains the only sellable and inventory-tracked unit with
SKU, barcode, price and currency. Composite foreign keys make every attached
option value belong to the same product as its variant. One value per option is
enforced per variant, while a deferred PostgreSQL constraint trigger rejects a
duplicate canonical option-value combination at transaction commit. Product
hydration loads options, values and variant selections only for detail reads;
the public localized slug endpoint exposes an availability matrix without
leaking stock quantities. Audited RBAC-protected Admin Facade commands create
options/variants, initialize internal stock through the configured warehouse,
and append both audit and Search product-change outbox events in one local
transaction. Checkout's variant-based contract remains unchanged.

## Phase 20 — Returns / RMA and controlled refunds

**Status: in progress (steps 1–2 complete).** The optional `returns` module owns
return requests, return items and an immutable status journal. It references
Orders, Identity and Catalog only through stable UUID integration identities;
there are no cross-module foreign keys. Its domain aggregate has an explicit
RMA lifecycle (`new → approved/rejected → received → refunded/closed`) that is
separate from both the payment workflow and warehouse restocking. A narrow
Orders snapshot port and configurable `RETURN_WINDOW` eligibility policy allow
the next step to enforce delivered-order and customer ownership rules before
any refund or inventory action is attempted. Customer creation and
permission-gated Admin approval/receipt APIs use the RMA aggregate. Receiving
atomically appends `returns.status_changed.v1` and a settlement command to the
Outbox; its worker performs idempotent unopened-item restock and starts a full
gateway refund only after commit. The request changes to `refunded` only after
the existing verified `orders.refunded.v1` event is delivered back to Returns.
Partial refunds and store credit remain explicitly unsupported by automatic
settlement until their pricing/credit policies are implemented.

## Global rules

- Do not fork for a store.
- Do not hardcode provider, locale, currency, tax or status rules in core.
- Do not store translatable business content in JSONB.
- Do not add vertical fields to core tables; create a module-owned table.
- Do not call a payment, carrier or ERP from inside a DB transaction.
- Update this roadmap, `ARCHITECTURE.md`, `.env.example` and API docs whenever
  a supported module or configuration capability changes.
