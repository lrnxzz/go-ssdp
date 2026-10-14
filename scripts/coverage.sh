#!/usr/bin/env bash
# Prints the coverage of each function, lowest first, and writes coverage.html. Runs the tests first
# when there is no profile yet.
set -euo pipefail

cd "$(dirname "$0")/.."

profile="${COVERAGE_PROFILE:-coverage.out}"

if [[ ! -f "$profile" ]]; then
	scripts/test.sh
fi

go tool cover -func="$profile" | sort -k3 -n | grep -v '^total:'
go tool cover -func="$profile" | grep '^total:'
go tool cover -html="$profile" -o coverage.html
