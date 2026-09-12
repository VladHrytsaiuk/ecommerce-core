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
