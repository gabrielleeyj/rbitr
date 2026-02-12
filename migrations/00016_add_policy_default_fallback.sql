-- +goose Up
-- Add a proper default fallback that handles missing input fields
-- This allows policy simulation with minimal inputs

UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {},
    "tags": []
}

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

# Main decision logic with proper else chaining
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
$$
WHERE tenant_id = 't_demo' AND policy_id = 'policy_demo';

UPDATE rbitr.policy_versions
SET rego_module = $$
package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {},
    "tags": []
}

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

# Main decision logic with proper else chaining
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
$$
WHERE tenant_id = 't_demo' AND policy_version = 'p_v1';

-- +goose Down
-- Revert to the version without the extra field checks
UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {},
    "tags": []
}

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

decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
    deny_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
    require_approval_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
    allow_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "HIGH"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_critical_risk_unknown", 80, "CRITICAL_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "CRITICAL"
} else := decision_obj("DENY", input.action_risk, "rule_default_deny", 100, "DEFAULT_DENY", "Default deny: no matching rule")
$$
WHERE tenant_id = 't_demo' AND policy_id = 'policy_demo';

UPDATE rbitr.policy_versions
SET rego_module = $$
package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {},
    "tags": []
}

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

decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
    deny_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
    require_approval_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
    allow_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "HIGH"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_critical_risk_unknown", 80, "CRITICAL_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
    input.action_risk == "CRITICAL"
} else := decision_obj("DENY", input.action_risk, "rule_default_deny", 100, "DEFAULT_DENY", "Default deny: no matching rule")
$$
WHERE tenant_id = 't_demo' AND policy_version = 'p_v1';
