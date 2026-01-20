-- +goose Up
UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

default decision := {
    "decision": "DENY",
    "rule_id": "rule_default_deny",
    "reason": "Default deny",
    "policy_version": "p_v1"
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

decision := {
    "decision": "ALLOW",
    "rule_id": "rule_allow_basic_actions_v1",
    "reason": "Policy: allow basic actions",
    "policy_version": "p_v1"
} if {
    allow_actions[input.action_type]
} else := {
    "decision": "REQUIRE_APPROVAL",
    "rule_id": "rule_require_approval_v1",
    "reason": "Policy: approval required",
    "policy_version": "p_v1"
} if {
    require_approval_actions[input.action_type]
} else := {
    "decision": "DENY",
    "rule_id": "rule_deny_sensitive_v1",
    "reason": "Policy: deny sensitive action",
    "policy_version": "p_v1"
} if {
    deny_actions[input.action_type]
}
$$
WHERE policy_id = 'policy_demo' AND tenant_id = 't_demo';

-- +goose Down
UPDATE rbitr.policies
SET rego_module = $$
package rbitr.policy

default decision = {
    "decision": "DENY",
    "rule_id": "rule_default_deny",
    "reason": "Default deny",
    "policy_version": "p_v1"
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

decision := {
    "decision": "ALLOW",
    "rule_id": "rule_allow_basic_actions_v1",
    "reason": "Policy: allow basic actions",
    "policy_version": "p_v1"
} {
    allow_actions[input.action_type]
} else := {
    "decision": "REQUIRE_APPROVAL",
    "rule_id": "rule_require_approval_v1",
    "reason": "Policy: approval required",
    "policy_version": "p_v1"
} {
    require_approval_actions[input.action_type]
} else := {
    "decision": "DENY",
    "rule_id": "rule_deny_sensitive_v1",
    "reason": "Policy: deny sensitive action",
    "policy_version": "p_v1"
} {
    deny_actions[input.action_type]
}
$$
WHERE policy_id = 'policy_demo' AND tenant_id = 't_demo';
