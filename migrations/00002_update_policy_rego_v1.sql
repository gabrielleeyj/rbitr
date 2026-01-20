-- +goose Up
UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {}
}

default decision := decision_obj("DENY", input.action_risk, "rule_default_deny", 100, "DEFAULT_DENY", "Default deny")

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
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high risk") if {
    input.action_risk == "HIGH" or input.action_risk == "CRITICAL"
}
$$
WHERE policy_id = 'policy_demo' AND tenant_id = 't_demo';

-- +goose Down
UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

decision_obj(decision, risk, rule_id, priority, code, message) := {
    "version": "2026-01-20",
    "decision": decision,
    "risk": risk,
    "rule": {"id": rule_id, "priority": priority},
    "reasons": [{"code": code, "message": message}],
    "constraints": {}
}

default decision := decision_obj("DENY", input.action_risk, "rule_default_deny", 100, "DEFAULT_DENY", "Default deny")

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
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high risk") if {
    input.action_risk == "HIGH" or input.action_risk == "CRITICAL"
}
$$
WHERE policy_id = 'policy_demo' AND tenant_id = 't_demo';
