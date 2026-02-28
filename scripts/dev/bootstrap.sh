#!/usr/bin/env bash
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
TENANT_NAME="${TENANT_NAME:-Demo Tenant}"
TENANT_ID="${TENANT_ID:-t_demo}"
ADMIN_KEY="${ADMIN_KEY:-admin_demo_key}"
TENANT_KEY="${TENANT_KEY:-tenant_demo_key}"

pretty() {
	if command -v jq >/dev/null 2>&1; then
		echo "$1" | jq
	else
		echo "$1"
	fi
}

setup_required() {
	local payload="$1"
	if command -v jq >/dev/null 2>&1; then
		echo "$payload" | jq -r '.setup_required // "false"'
	else
		if echo "$payload" | tr -d '\n' | grep -q '"setup_required":[[:space:]]*true'; then
			echo "true"
		else
			echo "false"
		fi
	fi
}

echo "Checking setup status from $GATEWAY_URL/setup/status ..."
status="$(curl -sS "$GATEWAY_URL/setup/status")"
if [ "$(setup_required "$status")" != "true" ]; then
	echo "Setup already completed; nothing to do."
	exit 0
fi

echo "Running first-time setup bootstrap ..."
init_payload="$(cat <<EOF
{
  "tenant_name": "${TENANT_NAME}",
  "tenant_id": "${TENANT_ID}",
  "admin_key": "${ADMIN_KEY}",
  "tenant_key": "${TENANT_KEY}"
}
EOF
)"

response="$(curl -sS -X POST "$GATEWAY_URL/setup/initialize" \
	-H "Content-Type: application/json" \
	-d "$init_payload")"

echo "Setup response:"
pretty "$response"

echo "Done. Tenant: $TENANT_ID"
