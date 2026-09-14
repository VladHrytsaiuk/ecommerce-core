# domain_events retention

Status: **decided 2026-09-14.** Financial topics are kept forever; retention for
`cart.updated.v1` is paused until external readers are inventoried; erasure of
personal data in events is a separate project —
see [domain-events-erasure.md](domain-events-erasure.md).

Nothing is built yet, and that is deliberate. The only topic a retention worker
would act on is paused, and a mechanism with no enabled topic is configuration
surface with no user. The latency half of the problem is already fixed:
`migrations/core/000022` indexes `(topic, aggregate_id, occurred_at DESC)`.

## Decision 1 — `orders.paid.v1` and `orders.refunded.v1` are kept forever

They are the primary source for the financial projections. Reports rebuilds
daily sales and refunds from them, so a deleted event does not raise an error:
it produces a quietly wrong report for that period. They are also few — one per
order, not one per cart action — so keeping them costs little.

**This is an allow-list of exactly these two topics.** It is not a rule that
`domain_events` are kept forever, and it must not be implemented as a default
that other topics fall into.

Two requirements follow:

- The protection is enforced in code, not only by leaving configuration empty.
  A configured window for a protected topic — `orders.paid.v1=90d` — is refused
  at startup, the same way an unknown `ENABLED_MODULES` name is.
- `maxRebuildDays = 366` (`internal/reports/application/rebuilder.go`) stays the
  bound on a single rebuild run. A multi-year rebuild is run as consecutive
  yearly ranges.

## Decision 2 — `cart.updated.v1`: paused until external readers are inventoried

Nothing in this repository reads the topic once its delivery is archived. That
is not the same as nothing reading it: a direct PostgreSQL reader — BI, ETL, a
read replica, change-data-capture — is invisible from the code.

Order of work:

1. Inventory readers, below.
2. **None found** — a 90-day window is acceptable.
3. **Readers found** — move each onto an explicit export or outbox sink,
   backfill it, switch it over, and only then enable deletion.

### Inventory checklist

Outside the database: BI and dashboard connections, ETL and warehouse jobs, cron
jobs on every host, other deployments sharing this database, read replicas wired
to reporting tools.

In the production database:

```sql
-- Who is allowed to read the table.
SELECT grantee, privilege_type
FROM information_schema.role_table_grants
WHERE table_name = 'domain_events'
ORDER BY grantee;

-- Every role that can log in. Anything besides the application is a candidate.
SELECT rolname, rolsuper, rolreplication
FROM pg_roles
WHERE rolcanlogin
ORDER BY rolname;

-- Who has actually queried it (requires pg_stat_statements).
SELECT r.rolname, s.calls, left(s.query, 120) AS query
FROM pg_stat_statements AS s
JOIN pg_roles AS r ON r.oid = s.userid
WHERE s.query ILIKE '%domain_events%'
ORDER BY s.calls DESC;

-- Readers that never issue a query: change-data-capture and replicas.
SELECT slot_name, plugin, slot_type, active FROM pg_replication_slots;
SELECT * FROM pg_publication_tables WHERE tablename = 'domain_events';
SELECT application_name, client_addr, state FROM pg_stat_replication;
```

The last three matter as much as the rest. A logical replication slot or a
physical replica feeding a reporting tool reads every row and leaves nothing in
`pg_stat_statements`.

## Decision 3 — erasure reaches `domain_events`, as a separate project

See [domain-events-erasure.md](domain-events-erasure.md). It does not block
retention and must not be folded into it.

## Deletion rules for any retention enabled later

A row may be deleted only when **all** of these hold:

- Its topic has a configured window, is not on the protected list, and
  `occurred_at` is past that window.
- **No row in `event_deliveries` references it, in any state.** This cannot be
  left to the database: `event_deliveries.event_id` references
  `domain_events(id)` with `ON DELETE CASCADE` (`migrations/core/000005`).
  Deleting an event that still has a pending delivery would not fail — it would
  silently delete the unprocessed delivery, and for `cart.updated.v1` that is a
  recovery campaign which never ran. With a 90-day window and a 30-day
  `OUTBOX_DONE_RETENTION`, completed deliveries are archived long before; an
  event whose delivery is dead or failed is therefore never purged, which is
  correct — an operator may still need to recover it.
- No `audit_logs` row references it. That foreign key is `ON DELETE RESTRICT`,
  so this one fails loudly, but the purge excludes such rows rather than
  discovering them through errors.
- Deleted in bounded batches, oldest first, inside the existing outbox retention
  worker, with a per-topic counter of what was removed.
