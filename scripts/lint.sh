#!/usr/bin/env bash
# Lints and checks formatting with the configuration in .golangci.yml.
set -euo pipefail

cd "$(dirname "$0")/.."

golangci-lint run ./...
