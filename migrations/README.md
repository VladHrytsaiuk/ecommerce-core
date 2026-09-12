# Migrations

Each directory is an independent `golang-migrate` sequence with its own
version table: `migrations/core` always runs, and each directory under
`migrations/modules` runs only when its module is in `ENABLED_MODULES`.

## Numbering

Numbers are per directory and must never be reused or renumbered. Two
directories have gaps — `consent` runs 1, 3 and `orders` runs 2, 3 — and those
gaps are original: no migration was ever deleted there, so nothing is missing
and no rollback path is broken. `golang-migrate` tracks the last applied
version rather than contiguity, so a gap is inert.

Renumbering to close one would not be: a deployment that has applied version 3
records exactly that, and a file renamed to 2 would be treated as unapplied
while 3 no longer exists. Leave the numbers where they are and add the next
one.

## Editing an applied migration

Don't. `golang-migrate` records only the version, not the file's contents, so
an edit reaches new deployments and silently skips every existing one, and the
two then disagree about the schema while both report the same version. Add a
migration that makes the change instead.

## Down migrations

Every `.up.sql` has a paired `.down.sql`; the pairing is checked. A down
migration that cannot restore the previous state — one whose up dropped data —
should say so in a comment rather than pretend.

## Retention

Which tables grow without bound is a decision, not an accident, so it is
recorded here.

**Retained forever, deliberately.** `orders`, `order_items`, `payments`,
`order_status_history` and `audit_logs` are financial or forensic records. They
are never pruned, and the cost of that is paid with indexes rather than
deletion — see `order_items_order_idx`.

**Pruned.** `domain_events` and `event_deliveries` have a retention worker with
a configurable window (`OUTBOX_DONE_RETENTION`). Terminal deliveries are
archived once nothing can need them for replay.

**Growing, undecided.** Two tables accumulate operational rows with no window:

- `inventory_reservations` keeps every terminal reservation — released,
  committed, expired, release_failed — one row per checkout line including
  abandoned checkouts, which makes it the fastest grower here. A committed
  reservation's evidence also lives in the order, so a window is defensible,
  but it must outlive any dispute or chargeback period the store is subject to;
  `release_failed` rows are an operator alert and must not be pruned at all.
- `notification_attempts` keeps one row per delivery attempt including retries.
  A window is defensible for the same reason, and must outlive the support
  window during which someone may ask why an email did not arrive.

Neither has been given a window yet: the right length is a store policy rather
than a property of this core, and pruning on the wrong one destroys evidence.
Until a deployment sets one, both are covered by the indexes their hot lookups
need, so size costs storage rather than latency.
