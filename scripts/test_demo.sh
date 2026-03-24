#!/usr/bin/env bash
set -euo pipefail

echo "=== rbitr Demo Test Script ==="
echo ""

# Configuration
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
TENANT_ID="${TENANT_ID:-t_acme}"
TENANT_NAME="${TENANT_NAME:-Demo Tenant}"
TENANT_KEY="${TENANT_KEY:-rbtr_live_o1rGiOw1-LLCsh8mQbJOy3ss3i2pjS-I5tMqwbHt26w}"
ADMIN_KEY="${ADMIN_KEY:-rbtr_admin_IWCpZwIwR4pYpuT-k5_mleli1UKz7TiF4T8q0g4aEEE}"
AGENT_ID="${AGENT_ID:-agent_demo}"
TOOL_ID="${TOOL_ID:-mock_internal}"
TENANT_AUTH_MODE="${TENANT_AUTH_MODE:-bearer}" # bearer|x-tenant-key
AUTO_RECOVER_TENANT_KEY="${AUTO_RECOVER_TENANT_KEY:-true}"
SETUP_TOKEN="${SETUP_TOKEN:-${RBTR_SETUP_TOKEN:-}}"

pretty() {
	if command -v jq >/dev/null 2>&1; then
		echo "$1" | jq .
	else
		echo "$1"
	fi
}

json_field() {
	local json="$1"
	local jq_expr="$2"
	local key="$3"
	if command -v jq >/dev/null 2>&1; then
		echo "$json" | jq -r "$jq_expr // empty"
	else
		echo "$json" | tr -d '\n' | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p"
	fi
}

response_is_unauthorized() {
	local json="$1"
	local err_field
	err_field="$(json_field "$json" '.error' 'error')"
	[ "$err_field" = "unauthorized" ]
}

tenant_auth_headers=()
set_tenant_auth_headers() {
	case "$TENANT_AUTH_MODE" in
	bearer)
		tenant_auth_headers=(-H "Authorization: Bearer $TENANT_KEY")
		;;
	x-tenant-key)
		tenant_auth_headers=(-H "X-Tenant-Key: $TENANT_KEY")
		;;
	*)
		echo "Invalid TENANT_AUTH_MODE: $TENANT_AUTH_MODE (use bearer or x-tenant-key)"
		exit 1
		;;
	esac
}
set_tenant_auth_headers

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

bootstrap_if_needed() {
	local status
	status="$(curl -sS "$GATEWAY_URL/setup/status" || true)"
	if [ -z "$status" ]; then
		return 0
	fi
	if [ "$(setup_required "$status")" != "true" ]; then
		return 0
	fi

	echo "Bootstrap is required. Running setup initialize for demo defaults..."
	local init_payload init_response
	init_payload="$(
		cat <<EOF
{"tenant_name":"$TENANT_NAME","tenant_id":"$TENANT_ID","admin_key":"$ADMIN_KEY","tenant_key":"$TENANT_KEY"}
EOF
	)"
	local setup_args
	setup_args=(
		-sS
		-X POST
		"$GATEWAY_URL/setup/initialize"
		-H "Content-Type: application/json"
		-H "Idempotency-Key: setup-$(date +%s%N)"
		-d "$init_payload"
	)
	if [ -n "$SETUP_TOKEN" ]; then
		setup_args+=(-H "Authorization: Bearer $SETUP_TOKEN")
	fi
	init_response="$(curl "${setup_args[@]}")"
	pretty "$init_response"
}

call_tool() {
	curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
		-H "Content-Type: application/json" \
		"${tenant_auth_headers[@]}" \
		-H "X-Agent-Id: $AGENT_ID" \
		-d "$1"
}

recover_tenant_key_if_needed() {
	local probe_response="$1"
	if ! response_is_unauthorized "$probe_response"; then
		return 0
	fi
	if [ "$AUTO_RECOVER_TENANT_KEY" != "true" ]; then
		return 1
	fi

	echo "Tenant key unauthorized. Attempting demo auto-recovery via admin key..."

	# Best-effort: ensure demo tenant is enabled.
	curl -sS -X PUT "$GATEWAY_URL/admin/tenants/$TENANT_ID/enabled" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $ADMIN_KEY" \
		-d '{"enabled":true}' >/dev/null || true

	local rotate_response
	rotate_response=$(curl -sS -X POST "$GATEWAY_URL/admin/tenants/$TENANT_ID/keys/rotate" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $ADMIN_KEY")
	local new_key
	new_key="$(json_field "$rotate_response" '.api_key' 'api_key')"

	if [ -z "$new_key" ]; then
		echo "Failed to rotate demo tenant key automatically."
		echo "Rotate response:"
		pretty "$rotate_response"
		echo "Set TENANT_KEY manually, or verify ADMIN_KEY has rotate scope."
		return 1
	fi

	TENANT_KEY="$new_key"
	set_tenant_auth_headers
	echo "Obtained new tenant key for $TENANT_ID. Continuing demo..."
	return 0
}

allow_payload='{"http_method":"GET","path":"/status","query":"","headers":{"Accept":"application/json"},"body":""}'
refund_payload='{"http_method":"POST","path":"/refund","query":"","headers":{"Content-Type":"application/json"},"body":"{\"amount\":100}"}'
deny_payload='{"http_method":"POST","path":"/export_customer_data","query":"","headers":{"Content-Type":"application/json"},"body":"{}"}'

bootstrap_if_needed

echo "1. Testing DATA.READ (should ALLOW)"
echo "-----------------------------------"
allow_response=$(call_tool "$allow_payload")
if ! recover_tenant_key_if_needed "$allow_response"; then
	pretty "$allow_response"
	echo ""
	echo "Unable to continue demo. Check TENANT_KEY / ADMIN_KEY and tenant status."
	exit 1
fi
if response_is_unauthorized "$allow_response"; then
	allow_response=$(call_tool "$allow_payload")
fi
pretty "$allow_response"
echo ""

echo "2. Testing PAYMENT.REFUND (should REQUIRE_APPROVAL)"
echo "---------------------------------------------------"
REFUND_RESPONSE=$(call_tool "$refund_payload")

pretty "$REFUND_RESPONSE"
echo ""

# Extract approval details
APPROVAL_REQUEST_ID="$(json_field "$REFUND_RESPONSE" '.approval_request_id' 'approval_request_id')"
APPROVAL_TOKEN="$(json_field "$REFUND_RESPONSE" '.approval_token' 'approval_token')"

if [ -z "$APPROVAL_REQUEST_ID" ]; then
	if response_is_unauthorized "$REFUND_RESPONSE"; then
		echo "Still unauthorized after recovery attempt. Stopping."
	else
		echo "No approval request created. Stopping."
	fi
	exit 1
fi

echo "3. Admin approves the request"
echo "-----------------------------"
approve_response=$(curl -sS -X POST "$GATEWAY_URL/admin/tenants/$TENANT_ID/approvals/$APPROVAL_REQUEST_ID/approve" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer $ADMIN_KEY" \
	-d '{"comment":"Approved by demo script"}')
pretty "$approve_response"
echo ""

echo "4. Agent resubmits with approval headers (should EXECUTE)"
echo "---------------------------------------------------------"
approved_response=$(curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
	-H "Content-Type: application/json" \
	"${tenant_auth_headers[@]}" \
	-H "X-Agent-Id: $AGENT_ID" \
	-H "X-Approval-Request-Id: $APPROVAL_REQUEST_ID" \
	-H "X-Approval-Token: $APPROVAL_TOKEN" \
	-d "$refund_payload")
pretty "$approved_response"
echo ""

echo "5. Testing DATA.EXPORT (should DENY)"
echo "------------------------------------"
deny_response=$(call_tool "$deny_payload")
pretty "$deny_response"
echo ""

echo "6. Creating a PENDING approval for manual testing"
echo "-------------------------------------------------"
pending_refund_payload='{"http_method":"POST","path":"/refund","query":"","headers":{"Content-Type":"application/json"},"body":"{\"amount\":250,\"reason\":\"customer_request\"}"}'
PENDING_RESPONSE=$(call_tool "$pending_refund_payload")
PENDING_APPROVAL_ID="$(json_field "$PENDING_RESPONSE" '.approval_request_id' 'approval_request_id')"
pretty "$PENDING_RESPONSE"
if [ -n "$PENDING_APPROVAL_ID" ]; then
	echo ""
	echo "  Pending approval ID: $PENDING_APPROVAL_ID"
	echo "  Approve it in the UI or via:"
	echo "    curl -X POST $GATEWAY_URL/admin/tenants/$TENANT_ID/approvals/$PENDING_APPROVAL_ID/approve \\"
	echo "      -H 'Content-Type: application/json' \\"
	echo "      -H 'Authorization: Bearer \$ADMIN_KEY' \\"
	echo "      -d '{\"comment\":\"Approved manually\"}'"
fi
echo ""

echo "7. Getting evidence trail"
echo "------------------------"
evidence_response=$(curl -sS "$GATEWAY_URL/v1/tenants/$TENANT_ID/evidence?limit=5" \
	"${tenant_auth_headers[@]}" \
	-H "X-Agent-Id: $AGENT_ID")
if command -v jq >/dev/null 2>&1; then
	echo "$evidence_response" | jq '.records[] | {decision, action_type, risk, rule_id, created_at}'
else
	echo "$evidence_response"
fi
echo ""

echo "--------| Demo complete! |---------"
