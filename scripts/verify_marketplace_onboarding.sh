#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-rbitr-marketplace-harness}"
SETUP_TOKEN="${SETUP_TOKEN:-setup_marketplace_token_2026}"
TENANT_ID="${TENANT_ID:-t_marketplace}"
TENANT_NAME="${TENANT_NAME:-Marketplace Tenant}"
ADMIN_KEY="${ADMIN_KEY:-admin_marketplace_key_2026!}"
TENANT_KEY="${TENANT_KEY:-tenant_marketplace_key_2026!}"
AGENT_ID="${AGENT_ID:-marketplace_harness_agent}"
WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-180}"
CURL_MAX_TIME_SECONDS="${CURL_MAX_TIME_SECONDS:-20}"
KEEP_COMPOSE_UP="${KEEP_COMPOSE_UP:-false}"
REPORT_FILE="${REPORT_FILE:-${REPO_ROOT}/artifacts/marketplace_onboarding_report.json}"
RBTR_SETUP_ALLOWED_CIDRS_HARNESS="${RBTR_SETUP_ALLOWED_CIDRS_HARNESS:-}"
COMPOSE_DATABASE_URL="${COMPOSE_DATABASE_URL:-postgres://postgres:postgres@db:5432/rbitr?sslmode=disable}"

OVERRIDE_FILE="$(mktemp "${TMPDIR:-/tmp}/rbitr-marketplace-compose-XXXXXX")"
RESP_STATUS=""
RESP_BODY=""

overall_result="failed"
failure_step=""
failure_reason=""

fresh_status_before_http="not_run"
fresh_setup_required_before="null"
fresh_initialize_without_token_http="not_run"
fresh_initialize_without_idempotency_http="not_run"
fresh_initialize_http="not_run"
fresh_initialize_replay_http="not_run"
fresh_initialize_conflict_http="not_run"
fresh_admin_me_http="not_run"
fresh_tenant_evidence_http="not_run"

upgrade_status_after_http="not_run"
upgrade_setup_required_after="null"
upgrade_initialize_http="not_run"
upgrade_admin_me_http="not_run"
upgrade_tenant_evidence_http="not_run"

compose() {
	docker compose \
		-p "${COMPOSE_PROJECT_NAME}" \
		-f "${REPO_ROOT}/docker-compose.yml" \
		-f "${OVERRIDE_FILE}" \
		"$@"
}

json_escape() {
	printf '%s' "$1" | tr '\n' ' ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

write_report() {
	local exit_code="$1"
	local report_dir
	local timestamp
	local escaped_reason
	local escaped_step
	local escaped_gateway
	local escaped_project
	local escaped_report_path

	report_dir="$(dirname "${REPORT_FILE}")"
	mkdir -p "${report_dir}"
	timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	escaped_reason="$(json_escape "${failure_reason}")"
	escaped_step="$(json_escape "${failure_step}")"
	escaped_gateway="$(json_escape "${GATEWAY_URL}")"
	escaped_project="$(json_escape "${COMPOSE_PROJECT_NAME}")"
	escaped_report_path="$(json_escape "${REPORT_FILE}")"

	cat >"${REPORT_FILE}" <<EOF
{
  "generated_at": "${timestamp}",
  "harness": "marketplace_onboarding",
  "overall_result": "${overall_result}",
  "exit_code": ${exit_code},
  "failure_step": "${escaped_step}",
  "failure_reason": "${escaped_reason}",
  "config": {
    "gateway_url": "${escaped_gateway}",
    "compose_project_name": "${escaped_project}",
    "report_file": "${escaped_report_path}",
    "setup_token_required": true,
    "setup_allowed_cidrs": "$(json_escape "${RBTR_SETUP_ALLOWED_CIDRS_HARNESS}")"
  },
  "fresh_install": {
    "status_http": "${fresh_status_before_http}",
    "setup_required": ${fresh_setup_required_before},
    "initialize_without_token_http": "${fresh_initialize_without_token_http}",
    "initialize_without_idempotency_http": "${fresh_initialize_without_idempotency_http}",
    "initialize_http": "${fresh_initialize_http}",
    "initialize_replay_http": "${fresh_initialize_replay_http}",
    "initialize_conflict_http": "${fresh_initialize_conflict_http}",
    "admin_me_http": "${fresh_admin_me_http}",
    "tenant_evidence_http": "${fresh_tenant_evidence_http}"
  },
  "upgrade_validation": {
    "status_http": "${upgrade_status_after_http}",
    "setup_required": ${upgrade_setup_required_after},
    "initialize_after_upgrade_http": "${upgrade_initialize_http}",
    "admin_me_http": "${upgrade_admin_me_http}",
    "tenant_evidence_http": "${upgrade_tenant_evidence_http}"
  }
}
EOF
}

abort() {
	failure_step="$1"
	failure_reason="$2"
	echo "ERROR [${failure_step}] ${failure_reason}" >&2
	exit 1
}

on_exit() {
	local exit_code="$?"

	if [ "${exit_code}" -eq 0 ]; then
		overall_result="passed"
	fi

	write_report "${exit_code}"

	if [ "${exit_code}" -ne 0 ]; then
		echo "Harness failed. Docker compose status:" >&2
		compose ps >&2 || true
		echo "Recent compose logs (gateway/migrate/db):" >&2
		compose logs --no-color --tail=120 gateway migrate db >&2 || true
	fi

	if [ "${KEEP_COMPOSE_UP}" != "true" ]; then
		compose down -v --remove-orphans >/dev/null 2>&1 || true
	fi

	rm -f "${OVERRIDE_FILE}" >/dev/null 2>&1 || true

	echo "Marketplace onboarding report written to: ${REPORT_FILE}"
}
trap on_exit EXIT

http_request() {
	local method="$1"
	local url="$2"
	local body="$3"
	shift 3

	local resp_file
	resp_file="$(mktemp "${TMPDIR:-/tmp}/rbitr-http-response-XXXXXX")"

	if [ -n "${body}" ]; then
		RESP_STATUS="$(curl -sS --max-time "${CURL_MAX_TIME_SECONDS}" -o "${resp_file}" -w "%{http_code}" -X "${method}" "${url}" "$@" -d "${body}")"
	else
		RESP_STATUS="$(curl -sS --max-time "${CURL_MAX_TIME_SECONDS}" -o "${resp_file}" -w "%{http_code}" -X "${method}" "${url}" "$@")"
	fi

	RESP_BODY="$(cat "${resp_file}")"
	rm -f "${resp_file}"
}

extract_setup_required() {
	local payload="$1"
	local value

	if command -v jq >/dev/null 2>&1; then
		value="$(printf '%s' "${payload}" | jq -r '.setup_required // empty' 2>/dev/null || true)"
		case "${value}" in
		true | false)
			printf '%s' "${value}"
			return 0
			;;
		esac
	fi

	if printf '%s' "${payload}" | tr -d '\n\r ' | grep -q '"setup_required":true'; then
		printf 'true'
		return 0
	fi
	if printf '%s' "${payload}" | tr -d '\n\r ' | grep -q '"setup_required":false'; then
		printf 'false'
		return 0
	fi

	printf 'null'
}

wait_for_gateway() {
	local deadline
	deadline="$((SECONDS + WAIT_TIMEOUT_SECONDS))"

	while [ "${SECONDS}" -lt "${deadline}" ]; do
		if curl -fsS --max-time 2 "${GATEWAY_URL}/healthz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done

	return 1
}

assert_status() {
	local step="$1"
	local expected="$2"
	local actual="$3"
	if [ "${actual}" != "${expected}" ]; then
		abort "${step}" "expected HTTP ${expected}, got ${actual}"
	fi
}

if ! command -v docker >/dev/null 2>&1; then
	abort "prereq_docker" "docker command is required"
fi
if ! command -v curl >/dev/null 2>&1; then
	abort "prereq_curl" "curl command is required"
fi
if [ ! -f "${REPO_ROOT}/docker-compose.yml" ]; then
	abort "prereq_compose_file" "docker-compose.yml not found at repo root"
fi

export RBTR_SETUP_TOKEN_HARNESS="${SETUP_TOKEN}"
export RBTR_SETUP_ALLOWED_CIDRS_HARNESS
export DATABASE_URL="${COMPOSE_DATABASE_URL}"

cat >"${OVERRIDE_FILE}" <<'EOF'
services:
  gateway:
    environment:
      RBTR_SETUP_TOKEN_REQUIRED: "true"
      RBTR_SETUP_TOKEN: "${RBTR_SETUP_TOKEN_HARNESS}"
      RBTR_SETUP_ALLOWED_CIDRS: "${RBTR_SETUP_ALLOWED_CIDRS_HARNESS}"
EOF

echo "Starting fresh install stack for marketplace onboarding verification..."
compose down -v --remove-orphans >/dev/null 2>&1 || true
if ! compose up -d --build db gateway; then
	abort "fresh_install_up" "failed to start docker compose stack"
fi

if ! wait_for_gateway; then
	abort "fresh_install_wait_gateway" "gateway did not become healthy within timeout"
fi

initialize_payload=$(cat <<EOF
{"tenant_name":"${TENANT_NAME}","tenant_id":"${TENANT_ID}","admin_key":"${ADMIN_KEY}","tenant_key":"${TENANT_KEY}"}
EOF
)

idempotency_key="marketplace-init-$(date +%s)-$$"

if ! http_request GET "${GATEWAY_URL}/setup/status" ""; then
	abort "fresh_status_call" "failed to call /setup/status"
fi
fresh_status_before_http="${RESP_STATUS}"
assert_status "fresh_status_call" "200" "${RESP_STATUS}"
fresh_setup_required_before="$(extract_setup_required "${RESP_BODY}")"
if [ "${fresh_setup_required_before}" != "true" ]; then
	abort "fresh_status_check" "expected setup_required=true before initialize"
fi

if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_payload}" \
	-H "Content-Type: application/json" \
	-H "Idempotency-Key: ${idempotency_key}-missing-token"; then
	abort "fresh_initialize_without_token" "failed to call /setup/initialize without token"
fi
fresh_initialize_without_token_http="${RESP_STATUS}"
assert_status "fresh_initialize_without_token" "401" "${RESP_STATUS}"

if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_payload}" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer ${SETUP_TOKEN}"; then
	abort "fresh_initialize_without_idempotency" "failed to call /setup/initialize without idempotency key"
fi
fresh_initialize_without_idempotency_http="${RESP_STATUS}"
assert_status "fresh_initialize_without_idempotency" "400" "${RESP_STATUS}"

if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_payload}" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer ${SETUP_TOKEN}" \
	-H "Idempotency-Key: ${idempotency_key}"; then
	abort "fresh_initialize" "failed to call /setup/initialize"
fi
fresh_initialize_http="${RESP_STATUS}"
assert_status "fresh_initialize" "201" "${RESP_STATUS}"
fresh_initialize_response="${RESP_BODY}"

if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_payload}" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer ${SETUP_TOKEN}" \
	-H "Idempotency-Key: ${idempotency_key}"; then
	abort "fresh_initialize_replay" "failed idempotent replay call"
fi
fresh_initialize_replay_http="${RESP_STATUS}"
assert_status "fresh_initialize_replay" "201" "${RESP_STATUS}"
if [ "${RESP_BODY}" != "${fresh_initialize_response}" ]; then
	abort "fresh_initialize_replay" "same idempotency key and payload did not return original result"
fi

initialize_conflict_payload=$(cat <<EOF
{"tenant_name":"${TENANT_NAME} Updated","tenant_id":"${TENANT_ID}","admin_key":"${ADMIN_KEY}","tenant_key":"${TENANT_KEY}"}
EOF
)
if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_conflict_payload}" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer ${SETUP_TOKEN}" \
	-H "Idempotency-Key: ${idempotency_key}"; then
	abort "fresh_initialize_conflict" "failed idempotency conflict call"
fi
fresh_initialize_conflict_http="${RESP_STATUS}"
assert_status "fresh_initialize_conflict" "409" "${RESP_STATUS}"

if ! http_request GET "${GATEWAY_URL}/admin/me" "" \
	-H "Authorization: Bearer ${ADMIN_KEY}"; then
	abort "fresh_admin_me" "failed to call /admin/me"
fi
fresh_admin_me_http="${RESP_STATUS}"
assert_status "fresh_admin_me" "200" "${RESP_STATUS}"

if ! http_request GET "${GATEWAY_URL}/v1/tenants/${TENANT_ID}/evidence?limit=1" "" \
	-H "Authorization: Bearer ${TENANT_KEY}" \
	-H "X-Agent-Id: ${AGENT_ID}"; then
	abort "fresh_tenant_evidence" "failed to call tenant evidence endpoint"
fi
fresh_tenant_evidence_http="${RESP_STATUS}"
assert_status "fresh_tenant_evidence" "200" "${RESP_STATUS}"

echo "Simulating upgrade by rebuilding and recreating gateway container..."
if ! compose up -d --build --no-deps gateway; then
	abort "upgrade_up" "failed to recreate gateway for upgrade simulation"
fi
if ! wait_for_gateway; then
	abort "upgrade_wait_gateway" "gateway did not recover after upgrade simulation"
fi

if ! http_request GET "${GATEWAY_URL}/setup/status" ""; then
	abort "upgrade_status_call" "failed to call /setup/status after upgrade"
fi
upgrade_status_after_http="${RESP_STATUS}"
assert_status "upgrade_status_call" "200" "${RESP_STATUS}"
upgrade_setup_required_after="$(extract_setup_required "${RESP_BODY}")"
if [ "${upgrade_setup_required_after}" != "false" ]; then
	abort "upgrade_status_check" "expected setup_required=false after upgrade"
fi

if ! http_request POST "${GATEWAY_URL}/setup/initialize" "${initialize_payload}" \
	-H "Content-Type: application/json" \
	-H "Authorization: Bearer ${SETUP_TOKEN}" \
	-H "Idempotency-Key: marketplace-upgrade-$(date +%s)-$$"; then
	abort "upgrade_initialize" "failed to call /setup/initialize after upgrade"
fi
upgrade_initialize_http="${RESP_STATUS}"
assert_status "upgrade_initialize" "409" "${RESP_STATUS}"

if ! http_request GET "${GATEWAY_URL}/admin/me" "" \
	-H "Authorization: Bearer ${ADMIN_KEY}"; then
	abort "upgrade_admin_me" "failed to call /admin/me after upgrade"
fi
upgrade_admin_me_http="${RESP_STATUS}"
assert_status "upgrade_admin_me" "200" "${RESP_STATUS}"

if ! http_request GET "${GATEWAY_URL}/v1/tenants/${TENANT_ID}/evidence?limit=1" "" \
	-H "Authorization: Bearer ${TENANT_KEY}" \
	-H "X-Agent-Id: ${AGENT_ID}"; then
	abort "upgrade_tenant_evidence" "failed to call tenant evidence endpoint after upgrade"
fi
upgrade_tenant_evidence_http="${RESP_STATUS}"
assert_status "upgrade_tenant_evidence" "200" "${RESP_STATUS}"

overall_result="passed"
echo "Marketplace onboarding verification completed successfully."
