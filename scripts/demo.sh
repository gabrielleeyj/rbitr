#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Keep make/demo compatibility by delegating to the maintained deploy demo flow.
"$SCRIPT_DIR/demo_deploy.sh"
