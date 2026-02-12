# rbitr Policy Template
#
# This is a reference template for creating policies in rbitr.
# All policies MUST include "import rego.v1" and follow this schema.

package rbitr.policy

import rego.v1

# Helper function to build decision objects
decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {},
    "tags": []
}

# Define action sets for easy classification
allow_actions := {
    "TICKET.CREATE",
    "TICKET.COMMENT",
    "TICKET.UPDATE",
    "CRM.READ",
    "DATA.READ",
    "DATA.QUERY"
}

require_approval_actions := {
    "PAYMENT.REFUND",
    "ACCESS.ROLE_CHANGE"
}

deny_actions := {
    "DATA.EXPORT",
    "DATA.BULK_EXPORT",
    "ACCESS.GRANT",
    "DATA.DELETE",
    "CRM.DELETE"
}

# Decision rules with proper else chaining
# Rules are evaluated in order, first match wins
# IMPORTANT: Check that input fields exist before using them (input.action_type, etc.)
decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
    input.action_type
    deny_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
    input.action_type
    require_approval_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
    input.action_type
    allow_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "HIGH"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_critical_risk_unknown", 80, "CRITICAL_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "CRITICAL"
} else := decision_obj("DENY", "MEDIUM", "rule_default_deny", 100, "DEFAULT_DENY", "Default deny: no matching rule or missing required fields")

# IMPORTANT NOTES:
#
# 1. MUST include "import rego.v1" - required by OPA
#
# 2. DO NOT use "default" with function calls - this is illegal in Rego:
#    ❌ default decision := decision_obj(...)
#    ✅ Use else chain with fallback at the end instead
#
# 3. Use "else :=" chaining to avoid rule conflicts:
#    ✅ decision := ... if { ... } else := ... if { ... }
#    ❌ decision := ... if { ... }
#       decision := ... if { ... }  # This causes conflicts!
#
# 4. Required output schema fields:
#    - version (string)
#    - decision (string: "ALLOW" | "DENY" | "REQUIRE_APPROVAL")
#    - risk (string: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL")
#    - rule.id (string)
#    - rule.priority (int)
#    - reasons (array of {code: string, message: string})
#    - constraints (object, can be empty {})
#    - tags (array, can be empty [])
#
# 5. Input available:
#    - input.tenant_id
#    - input.agent_id
#    - input.tool_id
#    - input.action_type
#    - input.action_risk
#    - input.request_hash
#    - input.policy_version
#
# 6. For approval constraints, use:
#    "constraints": {
#        "approval": {
#            "expires_in_seconds": 900,
#            "reason": "High-value transaction"
#        }
#    }
