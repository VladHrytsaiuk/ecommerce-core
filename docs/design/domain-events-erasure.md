# Erasure of personal data in domain_events

Status: **decided in principle 2026-09-14; separate project, not started.**
Implementation waits on a legal exception matrix — see Open work.

Related: [domain-events-retention.md](domain-events-retention.md). Retention and
erasure are different operations and are designed apart: retention removes old
rows on a schedule, erasure removes what identifies one person, on request.

## Decision

GDPR erasure covers `domain_events`.

The position "a UUID is not personal data" is rejected. In this system
`customer_id` links an event directly to a person, which makes it personal data.
GDPR covers indirectly identifiable persons, and pseudonymisation does not make
data anonymous while the controller is able to re-link it (GDPR Art. 4;
Recitals 26–28).

Legal input is needed not for whether these identifiers are personal data — in
this context the risk is clear — but for the exception matrix: which fields and
records must be retained under tax and accounting obligations. GDPR requires
data minimisation and also provides exceptions for legal obligations (Art. 5;
Art. 17).

## Design rules

- After an erasure is executed, remove `customer_id` and every other linkable
  identifier from the payloads of the affected topics.
- Do not replace an identifier with an HMAC or a stable pseudonym while the key
  or a mapping table stays with the controller. That is pseudonymisation, not
  erasure.
- Financial events stay available for rebuild, without customer linkage.
- The event recording the erasure itself is kept as a non-identifying audit
  fact — request ID, time, outcome, policy version — with no raw `customer_id`.
- Never rewrite an event while it has an active delivery. Complete, or safely
  cancel, its side effect first: rewriting mid-delivery changes what the
  consumer reads.

## What the payloads carry today

Inventoried from each topic's `MarshalPayload`. For the cart topics the
`aggregate_id` column is the cart id; for the order topics it is the order id.

| Topic | Identifiers in the payload | How it reaches a person |
| --- | --- | --- |
| `cart.updated.v1` | `cart_id`, `customer_id` (authenticated carts) | **Directly**, `customer_id` |
| `privacy.erasure_requested.v1` | `customer_id` | **Directly**, `customer_id` |
| `admin.action.v1` | `actor_user_id`, `ip_address`, `resource_id`, sanitised old/new payload, metadata | **Directly**, actor and IP; the old/new payload may hold customer data |
| `carts.created.v1` | `cart_id` | Through `carts` |
| `checkout.email_captured.v1` | `cart_id` — the email itself is not in the payload | Through the cart and `checkout_contacts` |
| `checkout.started.v1` | `order_id`, `cart_id` | Through `orders` |
| `orders.paid.v1` | `order_id`, `order_number`, amount, currency, time | Through `orders` |
| `orders.refunded.v1` | `order_id`, amount, currency, time | Through `orders` |
| `orders.status_changed.v1` | `order_id`, statuses, actor type | Through `orders` |
| `returns.status_changed.v1`, `returns.settlement_requested.v1` | `return_id`, `order_id` | Through `orders` |
| `catalog.product.changed.v1`, `inventory.variant.available.v1`, `media.asset.uploaded.v1`, `media.video.ready.v1` | product, variant or asset ids | None |

## Findings that shape the project

These came out of the inventory and are not yet decided.

**1. Most linkage is transitive, not in the payload.** Only three topics carry a
person's identifier directly. The rest carry an order or cart id, which
identifies a person only through `orders.customer_id` or `carts.customer_id`.
For those topics — both protected financial topics among them — erasure is
achieved by unlinking the `orders` and `carts` rows, not by rewriting events.
This is what makes "financial events are kept forever" compatible with erasure:
`orders.paid.v1` needs no payload change at all. It also means the event work
and whatever `ErasureExecutor` a store attaches must be designed together;
unlinking one without the other leaves the link in place.

**2. `privacy.erasure_requested.v1` writes the raw `customer_id` today**
(`internal/consent/domain/consent.go`), which contradicts the audit-fact rule
above. It is not a live exposure: no `ErasureExecutor` is wired, so approving an
erasure is refused and no such event can currently be written. It is an ordering
constraint — the payload must change before any store attaches an executor.

**3. `admin.action.v1` is the hardest case and needs the exception matrix most.**
It carries `actor_user_id` and `ip_address`, and `audit_logs` mirrors both with
`actor_user_id REFERENCES users(id) ON DELETE RESTRICT`. An administrator is a
person who can ask to be erased: an executor that deletes their `users` row
fails on that constraint, and an IP address is personal data in its own right.
Whether admin audit is retained under a legal-obligation exception, pseudonymised
or erased is exactly the question the matrix has to answer.

## Open work

1. Legal exception matrix, per topic and field and for `audit_logs`: retain,
   unlink, or erase.
2. Decide whether unlinking `orders` and `carts` and rewriting events happen in
   one transaction, and who owns it — the core or the store's executor.
3. Change the `privacy.erasure_requested.v1` payload to a non-identifying audit
   fact before any executor is wired.
