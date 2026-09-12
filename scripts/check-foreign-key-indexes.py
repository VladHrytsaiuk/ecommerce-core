#!/usr/bin/env python3
"""Fail when a foreign key has no index leading with its column.

PostgreSQL indexes the referenced side of a foreign key, never the referencing
side. Four such columns have already cost this project real time — two found by
an audit reading queries one at a time, two more found by this check on its
first run — and every one of them was a sequential scan on a path that runs
per order or per page view.

The rule: every REFERENCES column needs an index, primary key or unique
constraint whose FIRST column is it. A composite index only helps a lookup that
filters on its leading column, so UNIQUE (cart_id, variant_id) does nothing for
a lookup by variant_id alone.

Some columns genuinely never carry such a lookup. Those are listed in EXEMPT
with the reason, so an exemption is a decision on the record rather than an
omission.
"""

import re
import sys
from pathlib import Path

# table.column -> why no index is needed. An entry here is a claim that nothing
# filters, joins or cascades on this column often enough to matter; if that
# stops being true, delete the line rather than the check.
EXEMPT = {
    "cart_items.variant_id": "read by cart_id; the variant side is only an FK check, and variants are archived rather than deleted",
    "comparison_items.product_variant_id": "same as cart_items.variant_id",
    "wishlist_items.product_variant_id": "same as cart_items.variant_id",
    "order_items.variant_id": "joined from the variants side on their primary key; never filtered by here",
    "comparison_lists.category_id": "stored with the list and read with it, never filtered by",
    "inventory_reservations.warehouse_id": "always queried together with variant_id, which leads the lookup",
    "stock_items.warehouse_id": "covered by UNIQUE (variant_id, warehouse_id); every lookup supplies both",
    "product_videos.video_asset_id": "filtered only together with product_id, which leads the composite",
    "order_status_transitions.to_status_code": "a configuration table of a few dozen rows; the RESTRICT check on order_status_definitions scans nothing worth indexing, and lookups supply from_status_code first",
    # The seven currency columns reference supported_currencies(code), a static
    # ISO-4217 list seeded by migration 000011. Nothing filters money tables by
    # currency — every read is by order, customer or payment — and the only
    # work the referencing side would do is a child check when a currency row is
    # deleted, which is not an operation this system performs. Indexing a
    # CHAR(3) with a handful of distinct values across orders and payments would
    # cost writes on the money path and buy nothing.
    "orders.currency": "references the static supported_currencies list; money tables are never read by currency",
    "order_items.currency": "same as orders.currency",
    "payments.currency": "same as orders.currency",
    "payment_checkout_attempts.currency": "same as orders.currency",
    "payment_webhook_events.currency": "same as orders.currency",
    "payment_anomalies.currency": "same as orders.currency",
    "product_variants.currency": "same as orders.currency",
}


def load_sql(root: Path) -> str:
    sql = "\n".join(
        path.read_text() for path in sorted(root.glob("migrations/**/*.up.sql"))
    )
    # Strip line comments before anything parses this. A migration that explains
    # itself in prose — naming a table, or quoting the ALTER an operator should
    # run later — must not be read as schema. One that did produced a phantom
    # foreign key on a column called "then".
    return re.sub(r"--[^\n]*", "", sql)


def table_modules(root: Path) -> dict[str, str]:
    """Which migration directory declares each table."""
    owners: dict[str, str] = {}
    for path in sorted(root.glob("migrations/**/*.up.sql")):
        text = re.sub(r"--[^\n]*", "", path.read_text())
        for table in re.findall(r"CREATE TABLE\s+(\w+)\s*\(", text, re.I):
            owners[table.lower()] = str(path.parent)
    return owners


def unreferenced_children(sql: str, owners: dict[str, str], keys: set[tuple[str, str]]) -> list[str]:
    """Parent-child links inside one module that are not foreign keys.

    A column named <thing>_id, in a module that also declares the <thing>s table,
    is a reference to it. Across modules that link is deliberately left to the
    application — this is a modular monolith and a module does not constrain
    another's tables. Inside one module there is no such reason, and the absence
    is how a wrong identifier gets stored with no error: support_messages
    carried a zero ticket_id for the life of the table because nothing checked.
    """
    findings: list[str] = []
    for match in re.finditer(r"CREATE TABLE\s+(\w+)\s*\((.*?)\)\s*;", sql, re.S | re.I):
        table, body = match.group(1).lower(), match.group(2)
        for clause in body.split(","):
            clause = clause.strip()
            column = re.match(r"(\w+_id)\s+UUID", clause, re.I)
            if not column or re.search(r"\bREFERENCES\b", clause, re.I):
                continue
            name = column.group(1).lower()
            if (table, name) in keys:
                continue
            stem = name[:-3]
            # Tables are named with their module's prefix, so ticket_id in
            # support_messages points at support_tickets. Try the bare stem and
            # the stem under each prefix the child table itself carries.
            parts = table.split("_")
            prefixes = ["_".join(parts[:n]) for n in range(len(parts) - 1, 0, -1)]
            candidates = [stem + "s", stem]
            for prefix in prefixes:
                candidates += [f"{prefix}_{stem}s", f"{prefix}_{stem}"]
            for candidate in candidates:
                if candidate == table:
                    continue  # a stem that resolves back to its own table proves nothing
                if candidate in owners and owners[candidate] == owners.get(table):
                    findings.append(f"{table}.{name} -> {candidate}")
                    break
    return sorted(findings)


def foreign_keys(sql: str) -> set[tuple[str, str]]:
    found: set[tuple[str, str]] = set()
    for table_match in re.finditer(r"CREATE TABLE\s+(\w+)\s*\((.*?)\)\s*;", sql, re.S | re.I):
        table, body = table_match.group(1), table_match.group(2)
        for clause in body.split(","):
            clause = clause.strip()
            inline = re.match(r"(\w+)\s+[\w()\[\]]+.*?\bREFERENCES\b", clause, re.I | re.S)
            if inline:
                found.add((table, inline.group(1)))
            # The CONSTRAINT <name> prefix is optional and was not allowed for,
            # so every named table-level foreign key — including the composite
            # ones in the catalog module — was invisible to this check.
            table_level = re.match(
                r"(?:CONSTRAINT\s+\w+\s+)?FOREIGN KEY\s*\(\s*(\w+)", clause, re.I | re.S
            )
            if table_level:
                found.add((table, table_level.group(1)))
    for altered in re.finditer(
        r"ALTER TABLE\s+(\w+)[^;]*?ADD COLUMN\s+(\w+)[^;,]*?\bREFERENCES\b", sql, re.I | re.S
    ):
        found.add((altered.group(1), altered.group(2)))
    for constrained in re.finditer(
        r"ALTER TABLE\s+(\w+)[^;]*?\bFOREIGN KEY\s*\(\s*(\w+)[^;]*?\bREFERENCES\b", sql, re.I | re.S
    ):
        found.add((constrained.group(1), constrained.group(2)))
    return found


def has_leading_index(sql: str, table: str, column: str) -> bool:
    if re.search(
        r"CREATE\s+(UNIQUE\s+)?INDEX[^;]*?\bON\s+" + table + r"\s*\(\s*" + column + r"\b",
        sql, re.I | re.S,
    ):
        return True
    body = re.search(r"CREATE TABLE\s+" + table + r"\s*\((.*?)\)\s*;", sql, re.S | re.I)
    if not body:
        return False
    declarations = body.group(1)
    if re.search(r"\b" + column + r"\s+[\w()\[\]]+[^,]*\b(PRIMARY KEY|UNIQUE)\b", declarations, re.I):
        return True
    return bool(re.search(r"(PRIMARY KEY|UNIQUE)\s*\(\s*" + column + r"\b", declarations, re.I))


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    sql = load_sql(root)
    keys = foreign_keys(sql)
    if not keys:
        print("no foreign keys found; the pattern no longer matches the schema", file=sys.stderr)
        return 1

    missing = sorted(
        f"{table}.{column}"
        for table, column in keys
        if not has_leading_index(sql, table, column) and f"{table}.{column}" not in EXEMPT
    )
    stale = sorted(
        name
        for name in EXEMPT
        if name not in {f"{t}.{c}" for t, c in keys}
        or has_leading_index(sql, *name.split(".", 1))
    )

    if missing:
        print("These foreign keys have no index leading with their column:", file=sys.stderr)
        for name in missing:
            print(f"  {name}", file=sys.stderr)
        print(
            "\nPostgreSQL does not index the referencing side of a foreign key. Add an\n"
            "index, or add the column to EXEMPT in this script with the reason nothing\n"
            "looks it up.",
            file=sys.stderr,
        )
    if stale:
        print("\nThese exemptions are no longer needed and should be deleted:", file=sys.stderr)
        for name in stale:
            print(f"  {name}", file=sys.stderr)

    unconstrained = unreferenced_children(sql, table_modules(root), keys)
    if unconstrained:
        print(
            "\nThese columns reference a table in their own module but are not"
            " foreign keys:", file=sys.stderr,
        )
        for name in unconstrained:
            print(f"  {name}", file=sys.stderr)
        print(
            "\nWithin a module there is no reason to leave the link to the"
            " application.\nA wrong identifier written without one is stored"
            " silently and the row is\ninvisible from then on.",
            file=sys.stderr,
        )

    return 1 if missing or stale or unconstrained else 0


if __name__ == "__main__":
    sys.exit(main())
