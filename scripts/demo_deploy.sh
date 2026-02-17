#!/bin/sh
set -euo pipefail

GATEWAY_URL=${GATEWAY_URL:-"http://localhost:8080"}
TENANT_KEY=${TENANT_KEY:-"tenant_demo_key"}
AGENT_ID=${AGENT_ID:-"agent_demo"}
TENANT_ID=${TENANT_ID:-"t_demo"}
ADMIN_KEY=${ADMIN_KEY:-"admin_demo_key"}
TENANT_AUTH_MODE=${TENANT_AUTH_MODE:-"bearer"} # bearer|x-tenant-key
AUTO_RECOVER_TENANT_KEY=${AUTO_RECOVER_TENANT_KEY:-"true"}

if [ -f docker-compose.yml ] || [ -f compose.yaml ] || [ -f compose.yml ]; then
	echo "Starting docker compose services..."
	docker compose up -d
else
	echo "No docker compose file found; skipping compose startup."
fi

if command -v goose >/dev/null 2>&1 && [ -n "${DATABASE_URL:-}" ]; then
	echo "Running migrations..."
	goose -dir migrations postgres "$DATABASE_URL" up
else
	echo "Skipping migrations (missing goose or DATABASE_URL)."
fi

pretty() {
	if command -v jq >/dev/null 2>&1; then
		echo "$1" | jq
	else
		echo "$1"
	fi
}

json_field() {
	json="$1"
	jq_expr="$2"
	key="$3"
	if command -v jq >/dev/null 2>&1; then
		echo "$json" | jq -r "$jq_expr // empty"
	else
		echo "$json" | tr -d '\n' | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p"
	fi
}

response_is_unauthorized() {
	json="$1"
	err_field=$(json_field "$json" '.error' 'error')
	[ "$err_field" = "unauthorized" ]
}

set_tenant_auth_header() {
	tenant_auth_header_name="Authorization"
	tenant_auth_header_value="Bearer $TENANT_KEY"
	if [ "$TENANT_AUTH_MODE" = "x-tenant-key" ]; then
		tenant_auth_header_name="X-Tenant-Key"
		tenant_auth_header_value="$TENANT_KEY"
	fi
}
set_tenant_auth_header

call_tool() {
	curl -sS -X POST "$GATEWAY_URL/v1/tools/mock_internal/call" \
		-H "Content-Type: application/json" \
		-H "$tenant_auth_header_name: $tenant_auth_header_value" \
		-H "X-Agent-Id: $AGENT_ID" \
		-d "$1"
}

allow_payload='{"http_method":"GET","path":"/status","query":"","headers":{"Accept":"application/json"},"body":""}'
approval_payload='{"http_method":"POST","path":"/refund","query":"","headers":{"Content-Type":"application/json"},"body":"{\"amount\":100}"}'
deny_payload='{"http_method":"POST","path":"/export_customer_data","query":"","headers":{"Content-Type":"application/json"},"body":"{}"}'

printf "\n1) ALLOW (expected)\n"
allow_resp=$(call_tool "$allow_payload")
if response_is_unauthorized "$allow_resp" && [ "$AUTO_RECOVER_TENANT_KEY" = "true" ]; then
	echo "Tenant key unauthorized. Attempting demo auto-recovery via admin key..."
	curl -sS -X PUT "$GATEWAY_URL/admin/tenants/$TENANT_ID/enabled" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $ADMIN_KEY" \
		-d '{"enabled":true}' >/dev/null || true
	rotate_resp=$(curl -sS -X POST "$GATEWAY_URL/admin/tenants/$TENANT_ID/keys/rotate" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $ADMIN_KEY")
	new_key=$(json_field "$rotate_resp" '.api_key' 'api_key')
	if [ -n "$new_key" ]; then
		TENANT_KEY="$new_key"
		set_tenant_auth_header
		echo "Obtained new tenant key for $TENANT_ID."
		allow_resp=$(call_tool "$allow_payload")
	else
		echo "Failed to rotate demo tenant key automatically."
		pretty "$rotate_resp"
	fi
fi
pretty "$allow_resp"

printf "\n2) REQUIRE_APPROVAL (expected)\n"
approval_resp=$(call_tool "$approval_payload")
pretty "$approval_resp"

approval_id=""
approval_token=""
approval_id=$(json_field "$approval_resp" '.approval_request_id' 'approval_request_id')
approval_token=$(json_field "$approval_resp" '.approval_token' 'approval_token')

if [ -n "$approval_id" ] && [ -n "$approval_token" ]; then
	printf "\n2b) Admin approve\n"
	approve_resp=$(curl -sS -X POST "$GATEWAY_URL/admin/tenants/$TENANT_ID/approvals/$approval_id/approve" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $ADMIN_KEY" \
		-d '{"comment":"approved in demo"}')
	pretty "$approve_resp"

	printf "\n2c) Resubmit with approval token (expected ALLOW)\n"
	approved_resp=$(curl -sS -X POST "$GATEWAY_URL/v1/tools/mock_internal/call" \
		-H "Content-Type: application/json" \
		-H "$tenant_auth_header_name: $tenant_auth_header_value" \
		-H "X-Agent-Id: $AGENT_ID" \
		-H "X-Approval-Request-Id: $approval_id" \
		-H "X-Approval-Token: $approval_token" \
		-d "$approval_payload")
	pretty "$approved_resp"
else
	echo "Skipping approval execution (jq missing or approval token not returned)."
fi

printf "\n3) DENY (expected)\n"
deny_resp=$(call_tool "$deny_payload")
pretty "$deny_resp"

printf "\n4) Evidence Export\n"
evidence_resp=$(curl -sS "$GATEWAY_URL/v1/tenants/$TENANT_ID/evidence?limit=50" \
	-H "$tenant_auth_header_name: $tenant_auth_header_value" \
	-H "X-Agent-Id: $AGENT_ID")
pretty "$evidence_resp"

printf "\nEvidence Pack Preview Summary\n"
if command -v jq >/dev/null 2>&1; then
	echo "$evidence_resp" | jq -r '.records[] | "- " + .decision + " " + .action_type + " (" + .reason + ")"'
	echo ""
	echo "Approval metadata preview:"
	echo "$evidence_resp" | jq -r '.records[] | select(.approval_request_id != null) | "- " + .approval_request_id + " status=" + (.approval_status // "") + " decided_by=" + (.approval_decided_by // "")'
else
	echo "Install jq for a concise summary."
fi
