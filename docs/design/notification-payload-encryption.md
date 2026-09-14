# Encrypting every notification job

**Status:** stages 1-3 shipped. Stages 4 and 5 wait on a deployment having run
the backfill; their migration is parked in `docs/design/pending/` rather than in
`migrations/`, because applying it while the dual-read below is still in the
code would drop columns that code still reads.

That is the whole reason the work is staged, and writing all five stages at once
walked straight into it: on a fresh database the guard in stage 4 passes — there
is nothing to back fill — so the columns went immediately and every claim
failed. The migration only becomes safe once the release that removes the
dual-read is the one being deployed.

## What is open today

`notification_jobs` carries two kinds of row and encrypts only one of them.

| Dispatcher | Written by | Recipient | Render payload |
|---|---|---|---|
| `outbox` | order receipts | inside the ciphertext | AES-GCM in `payload_ciphertext` |
| `scheduler` | support replies, abandoned carts, back-in-stock, return status | plaintext in `recipient_email` | plaintext in `payload` |

Migration `000002` stated the invariant — "new notification jobs persist render
data only as authenticated ciphertext" — and `000004` reintroduced a plaintext
column for the scheduled path, which is now four of the five message types.

The scheduled payloads are not less sensitive than a receipt. A support reply
carries the agent's message body verbatim; a return notification carries
identifiers tied to one customer's order.

`dedupe_key` leaks the same data a third way: it is built as
`type + ":" + recipient + ":" + payload`, so the address and the rendered
content sit in an indexed column even for rows whose payload is encrypted.

## The contract

- Every new job stores the recipient and the render payload only in
  `payload_ciphertext`.
- `recipient_email`, `payload` and the legacy `recipient` column are dropped
  after a staged migration.
- `dedupe_key` becomes an HMAC-SHA-256 over a canonical `{type, recipient,
  payload}`, keyed by a dedupe secret of its own or a key derived through HKDF
  from the encryption key. No personal data in the key.
- Left in the clear, deliberately, because operating the queue requires them:
  `type`, `locale`, `status`, `dispatcher`, timestamps, and the sanitized error
  code.

## Staging

Each step ships and runs on its own; none of them is reversible by the next.

1. **Shipped.** Migration `000007` relaxes the constraint that required a
   scheduler job to carry a plaintext payload, so ciphertext can be written at
   all. It widens what is accepted and rejects nothing that was accepted before.
2. **Shipped.** `ScheduleEmail` writes the recipient and the payload only into
   `payload_ciphertext`, and refuses to write at all when encryption is not
   configured rather than falling back. `ClaimDue` reads either shape, so a job
   queued by the previous release is still sent. `dedupe_key` is an HMAC.
3. **Shipped.** `go run ./cmd/cli notifications-reencrypt` moves existing rows
   out of the plaintext columns in bounded batches and rewrites their dedupe
   keys, skipping rows the dispatcher holds. Safe to run repeatedly and while
   the store is serving.
4. **Pending.** The constraint that forbids plaintext.
5. **Pending.** Dropping `payload`, `recipient_email` and the legacy
   `recipient`.

Steps 4 and 5 are one migration, `docs/design/pending/000008_drop_notification_plaintext.*`.
Before moving it into `migrations/modules/notifications/`:

- the release removing the dual-read from `ClaimDue` must be the one deploying,
  or it will read columns that no longer exist;
- `notifications-reencrypt` must report nothing left in plaintext. The migration
  checks this itself and refuses otherwise, because those columns are the only
  copy of what they hold.

Step 3 runs through the application because the encryption key lives there: a
SQL backfill cannot produce ciphertext, and a migration that tried would have to
be given the key.

## Why retention shipped first

Retention is independent and bounded — it changes no format and no read path —
and it shortens the window during which the plaintext exists at all. Encryption
changes the shape of every write and needs the staged rollout above, so pairing
them would have put a reversible cleanup behind an irreversible migration.
