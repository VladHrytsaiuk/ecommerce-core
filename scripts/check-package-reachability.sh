#!/usr/bin/env bash

# A Go package that nothing imports still compiles, still passes vet, and still
# passes staticcheck's unused check — U1000 reasons within a package and says
# nothing about a package no entrypoint reaches. Three such packages had
# accumulated here, and two of them were admin route sets with no authorization
# and no audit trail: code that looked like a working feature, would have been
# a security hole if anyone had wired it, and nothing flagged.
#
# This fails the build when a package under internal/ is unreachable from every
# command in cmd/. Test-only helpers are not affected: this walks the non-test
# dependency graph, and a package used solely by tests is not in it — such a
# package must be a _test package or live beside the code it supports.

set -euo pipefail

module="$(go list -m)"

reachable="$(mktemp)"
present="$(mktemp)"
trap 'rm -f "$reachable" "$present"' EXIT

go list -deps ./cmd/... | sort > "$reachable"
go list ./... | grep "^${module}/internal/" | sort > "$present"

unreachable="$(comm -23 "$present" "$reachable" | sed "s|^${module}/||")"

if [ -n "$unreachable" ]; then
  echo 'These packages are not reachable from any command in cmd/:' >&2
  echo "$unreachable" | sed 's/^/  /' >&2
  echo >&2
  echo 'Wire the package into the composition root or delete it. An unreachable' >&2
  echo 'package is not dormant: it is code nobody runs and nobody reviews.' >&2
  exit 1
fi
