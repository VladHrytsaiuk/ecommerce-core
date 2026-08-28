#!/usr/bin/env bash

set -euo pipefail

active_paths=(
  cmd/api
  cmd/migrate
  internal/app
  internal/catalog
  internal/checkout
  internal/core
  internal/delivery
  internal/http
  internal/inventory
  internal/orders
  internal/payments
  internal/search
  internal/sync
  migrations/core
  migrations/modules
)

# User-profile schemas, sanitized Admin audit payloads, and immutable Orders
# workflow metadata deliberately use JSONB. None stores translatable business
# content; translations remain normalized everywhere else in the active graph.
if rg -n -i 'jsonb' migrations/core migrations/modules \
  --glob '!migrations/modules/user_profiles/**' \
  --glob '!migrations/modules/admin/**' \
  --glob '!migrations/modules/orders/**'; then
  echo 'Active migrations must not store multilingual content in JSONB.' >&2
  exit 1
fi

if rg -n 'LocalizedMap' "${active_paths[@]}"; then
  echo 'Active clean modules must not use legacy LocalizedMap.' >&2
  exit 1
fi

if rg -n 'map\[string\]string' internal/catalog; then
  echo 'Catalog translations must use normalized records, not maps.' >&2
  exit 1
fi

# Optional modules may reference stable identity/catalog entities (users,
# products, variants) for local referential integrity. They must not attach
# themselves to order workflows, which are a cross-context integration seam.
if rg -n 'REFERENCES orders' migrations/modules; then
  echo 'Module migrations must not declare foreign keys to Orders.' >&2
  exit 1
fi

if rg -n 'internal/(orders|inventory)/repository/postgres' internal/core; then
  echo 'Core must not import a concrete repository from another module.' >&2
  exit 1
fi

if rg -n 'github\.com/VladHrytsaiuk/ecommerce-core/internal/(category|product)(/|"|$)' "${active_paths[@]}"; then
  echo 'The active clean graph must not import removed legacy Catalog packages.' >&2
  exit 1
fi
