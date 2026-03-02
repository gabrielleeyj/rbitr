#!/usr/bin/env bash
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
TENANT_NAME="${TENANT_NAME:-Demo Tenant}"
TENANT_ID="${TENANT_ID:-t_demo}"
ADMIN_KEY="${ADMIN_KEY:-admin_demo_key_2026!}"
TENANT_KEY="${TENANT_KEY:-tenant_demo_key_2026!}"
SETUP_TOKEN="${SETUP_TOKEN:-${RBTR_SETUP_TOKEN:-}}"

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

curl_args=(
	-sS
	-X POST
	"$GATEWAY_URL/setup/initialize"
	-H "Content-Type: application/json"
	-H "Idempotency-Key: setup-$(date +%s%N)"
)
if [ -n "$SETUP_TOKEN" ]; then
	curl_args+=(-H "Authorization: Bearer $SETUP_TOKEN")
fi
curl_args+=(-d "$init_payload")
response="$(curl "${curl_args[@]}")"

echo "Setup response:"
pretty "$response"

echo "Done. Tenant: $TENANT_ID"
