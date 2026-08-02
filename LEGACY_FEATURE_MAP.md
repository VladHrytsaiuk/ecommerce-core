# Legacy feature map

`legacy-monolith-baseline` is the immutable reference snapshot for functionality
that existed before the clean-slate Catalog removal. It is **not** imported by
the running application. Before retiring another legacy package, record its
behaviour here and port its tests or scenarios to the owning clean module.

| Legacy capability | Reference at `legacy-monolith-baseline` | Target owner | Migration / acceptance focus | Status |
|---|---|---|---|---|
| Category hierarchy, localized names, slugs | `internal/category/` | `internal/catalog` | `categories`, `category_translations`; three-locale CRUD | Rebuilding |
| Product content and localized slugs | `internal/product/` | `internal/catalog` | `products`, `product_translations`; publish status | Rebuilding |
| Variants, SKU, barcode, prices | `internal/product/` | `internal/catalog` + `internal/inventory` | catalog-owned `product_variants`; inventory-owned stock and reservation migrations | Rebuilding |
| Images and localized alt text | `internal/product/` | `internal/media` | owned asset tables and storage port | Planned |
| Attributes, values and storefront filters | `internal/product/` | `internal/attributes` | normalized attribute/value translation tables | Planned |
| Brands, badges and bundles | `internal/product/` | `catalog` / `promotions` | decide module ownership before schema | Planned |
| Cart, guest sessions and price recalculation | `internal/cart/` | `internal/checkout` | clean cart workflow against variants port | Planned |
| Wishlist | `internal/wishlist/` | `internal/wishlist` | product/variant read-model port | Planned |
| Order creation and lifecycle | `internal/order/` | `internal/orders` | order snapshot, state transitions, outbox | Planned |
| Payment callbacks and provider state | `internal/payment/` | `internal/payments` + adapters | idempotent gateway events | Planned |
| Shipping quotes and tracking | `internal/shipment/`, `internal/shipping/` | `internal/delivery` + adapters | carrier port and shipment snapshot | Planned |

## Porting protocol

1. Read the reference implementation and its tests from the tag.
2. Write provider-neutral clean-module tests that express the retained behaviour.
3. Add only the module-owned migration and model required by those tests.
4. Register the completed module in `internal/app/bootstrap.go`.
5. Mark the row `Rebuilt` only after its tests pass and no active package imports
   the legacy path.
