#!/usr/bin/env bash
# Runs the unit tests and the integration tests, which start Gerbera, MiniDLNA and async-upnp-client
# in Docker, and writes one coverage profile for the library.
set -euo pipefail

cd "$(dirname "$0")/.."

profile="${COVERAGE_PROFILE:-coverage.out}"

go test -race -count=1 -timeout=15m -covermode=atomic -coverpkg=./ssdp -coverprofile="$profile" ./...
