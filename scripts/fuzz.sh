#!/usr/bin/env bash
# Fuzzes the decoder for FUZZ_TIME (30s by default): no datagram may make it panic, and whatever it
# decodes has to encode back to the same bytes.
set -euo pipefail

cd "$(dirname "$0")/.."

go test -run='^$' -fuzz='^FuzzDecode$' -fuzztime="${FUZZ_TIME:-30s}" ./ssdp
