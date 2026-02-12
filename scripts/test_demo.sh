#!/bin/bash
set -e

echo "=== rbitr Demo Test Script ==="
echo ""

# Configuration
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
TENANT_KEY="tenant_demo_key"
ADMIN_KEY="admin_demo_key"
AGENT_ID="agent_demo"
TOOL_ID="mock_internal"

echo "1. Testing DATA.READ (should ALLOW)"
echo "-----------------------------------"
curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Key: $TENANT_KEY" \
  -H "X-Agent-Id: $AGENT_ID" \
  -d '{"http_method":"GET","path":"/status","query":"","headers":{"Accept":"application/json"},"body":""}' \
  | jq .
echo ""

echo "2. Testing PAYMENT.REFUND (should REQUIRE_APPROVAL)"
echo "---------------------------------------------------"
REFUND_RESPONSE=$(curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Key: $TENANT_KEY" \
  -H "X-Agent-Id: $AGENT_ID" \
  -d '{"http_method":"POST","path":"/refund","query":"","headers":{"Content-Type":"application/json"},"body":"{\"amount\":100}"}')

echo "$REFUND_RESPONSE" | jq .
echo ""

# Extract approval details
APPROVAL_REQUEST_ID=$(echo "$REFUND_RESPONSE" | jq -r '.approval_request_id // empty')
APPROVAL_TOKEN=$(echo "$REFUND_RESPONSE" | jq -r '.approval_token // empty')

if [ -z "$APPROVAL_REQUEST_ID" ]; then
  echo "❌ No approval request created. Stopping."
  exit 1
fi

echo "3. Admin approves the request"
echo "-----------------------------"
curl -sS -X POST "$GATEWAY_URL/admin/tenants/t_demo/approvals/$APPROVAL_REQUEST_ID/approve" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -d '{"comment":"Approved by demo script"}' \
  | jq .
echo ""

echo "4. Agent resubmits with approval headers (should EXECUTE)"
echo "---------------------------------------------------------"
curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Key: $TENANT_KEY" \
  -H "X-Agent-Id: $AGENT_ID" \
  -H "X-Approval-Request-Id: $APPROVAL_REQUEST_ID" \
  -H "X-Approval-Token: $APPROVAL_TOKEN" \
  -d '{"http_method":"POST","path":"/refund","query":"","headers":{"Content-Type":"application/json"},"body":"{\"amount\":100}"}' \
  | jq .
echo ""

echo "5. Testing DATA.EXPORT (should DENY)"
echo "------------------------------------"
curl -sS -X POST "$GATEWAY_URL/v1/tools/$TOOL_ID/call" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Key: $TENANT_KEY" \
  -H "X-Agent-Id: $AGENT_ID" \
  -d '{"http_method":"POST","path":"/export_customer_data","query":"","headers":{"Content-Type":"application/json"},"body":"{}"}' \
  | jq .
echo ""

echo "6. Getting evidence trail"
echo "------------------------"
curl -sS "$GATEWAY_URL/v1/tenants/t_demo/evidence?limit=5" \
  -H "X-Tenant-Key: $TENANT_KEY" \
  -H "X-Agent-Id: $AGENT_ID" \
  | jq '.records[] | {decision, action_type, risk, rule_id, created_at}'
echo ""

echo "✅ Demo complete!"
