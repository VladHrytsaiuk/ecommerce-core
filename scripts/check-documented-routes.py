#!/usr/bin/env python3
"""Fail when README documents a route the API does not serve.

Twice now an audit has found README describing surfaces that were removed:
`/api/admin/...` and `/api/:lang/checkout/...` both kept their sections here
long after v1 replaced them, so an integrator following this file got a 404 on
the first request.

The generated OpenAPI contract is the reference. CI regenerates it from the code
and fails on any difference, so a path present there is a path the API serves.

README may write a concrete value where the contract writes a parameter —
`/api/webhooks/payments/liqpay` for `/api/webhooks/payments/{provider}` — so a
contract path matches with each `{param}` standing for one segment. That is
deliberately permissive: the failure worth catching is a section describing a
surface that no longer exists at all, not a worked example.
"""

import re
import sys
from pathlib import Path

# Paths README states as prefixes or shapes rather than as routes.
IGNORE_SUFFIXES = ("...", "*", "/")


def documented_paths(readme: str) -> set[str]:
    found = set(re.findall(r"`(?:GET|POST|PUT|PATCH|DELETE) (/api/[^`]*)`", readme))
    found |= set(re.findall(r"`(/api/[A-Za-z0-9_{}/:.-]+)`", readme))
    paths = {path.split("?")[0].rstrip("/") for path in found if not path.endswith(IGNORE_SUFFIXES)}
    # `/api/v1` and the like name the surface, not a route.
    return {path for path in paths if path.count("/") > 2}


def served_matchers(contract: str) -> list[re.Pattern[str]]:
    patterns = []
    for path in re.findall(r"^  (/api/[^:]*):$", contract, re.M):
        escaped = re.escape(path).replace(r"\{", "{").replace(r"\}", "}")
        patterns.append(re.compile("^" + re.sub(r"\{[^}]+\}", r"[^/]+", escaped) + "$"))
    return patterns


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    contract_file = root / "docs" / "api" / "swagger.yaml"
    if not contract_file.exists():
        print("docs/api/swagger.yaml is missing; run swag init", file=sys.stderr)
        return 1

    matchers = served_matchers(contract_file.read_text())
    if not matchers:
        print("no paths parsed from the contract; the pattern no longer matches it", file=sys.stderr)
        return 1

    missing = sorted(
        path
        for path in documented_paths((root / "README.md").read_text())
        if not any(matcher.match(path) for matcher in matchers)
    )
    if missing:
        print("README documents routes the API does not serve:", file=sys.stderr)
        for path in missing:
            print(f"  {path}", file=sys.stderr)
        print(
            "\nThe generated contract in docs/api/swagger.yaml is the reference.\n"
            "Either the route moved and README should say where, or the route is\n"
            "gone and its section should be too.",
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
