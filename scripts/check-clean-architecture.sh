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
  internal/sync
  migrations/core
  migrations/modules
)

if rg -n -i 'jsonb' migrations/core migrations/modules; then
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

if rg -n 'REFERENCES (product_variants|orders)' migrations/modules; then
  echo 'Module migrations must not declare foreign keys to Core tables.' >&2
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
