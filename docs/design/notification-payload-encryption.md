# Encrypting every notification job

**Status:** planned, not implemented. Retention (shipped) bounds how long the
plaintext below exists; it does not make it any less plaintext.

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

1. Add the new format alongside the old.
2. Dual-read and dual-write in the application.
3. Re-encrypt live rows in bounded batches, through the application.
4. Add the database constraint that forbids the plaintext columns being set.
5. Drop the old columns.

Step 3 runs through the application because the encryption key lives there: a
SQL backfill cannot produce ciphertext, and a migration that tried would have to
be given the key.

## Why retention shipped first

Retention is independent and bounded — it changes no format and no read path —
and it shortens the window during which the plaintext exists at all. Encryption
changes the shape of every write and needs the staged rollout above, so pairing
them would have put a reversible cleanup behind an irreversible migration.
