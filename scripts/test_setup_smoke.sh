#!/usr/bin/env bash
set -euo pipefail

echo "Running setup smoke tests (API + setup flow assertions)..."
go test ./internal/api/setup -v
