package public

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const (
	argConstraintReasonDeny       = "ARG_CONSTRAINT_DENY"
	argConstraintReasonNotAllowed = "ARG_CONSTRAINT_NOT_ALLOWED"
	argConstraintReasonInvalid    = "ARG_CONSTRAINT_INVALID_RULE"

	argConstraintDefaultDenyMessage       = "Argument value denied by policy constraint"
	argConstraintDefaultNotAllowedMessage = "Argument value not allowed by policy constraint"
	argConstraintDefaultInvalidMessage    = "Argument constraint rule is invalid"

	argConstraintOpEq         = "eq"
	argConstraintOpPrefix     = "prefix"
	argConstraintOpRegex      = "regex"
	argConstraintOpIn         = "in"
	argConstraintOpContains   = "contains"
	argConstraintOpJSONSchema = "jsonschema"
)

type argConstraintRule struct {
	ID      string
	Path    string
	Op      string
	Value   any
	Message string
}

type argConstraintRuleSet struct {
	Allow []argConstraintRule
	Deny  []argConstraintRule
}

type argConstraintFailure struct {
	Path       string
	Op         string
	ReasonCode string
	RuleID     string
}

type argConstraintViolation struct {
	ReasonCode string
	Message    string
	Failures   []argConstraintFailure
}

func (d *Dependencies) enforceArgumentConstraints(ctx context.Context, constraints map[string]any, arguments any) *argConstraintViolation {
	if !d.featureArgConstraintsEnabled(ctx) {
		return nil
	}

	ruleSet, ok := parseArgConstraintRuleSet(constraints)
	if !ok {
		return &argConstraintViolation{
			ReasonCode: argConstraintReasonInvalid,
			Message:    argConstraintDefaultInvalidMessage,
		}
	}
	if len(ruleSet.Deny) == 0 && len(ruleSet.Allow) == 0 {
		return nil
	}

	var denyFailures []argConstraintFailure
	firstDenyMessage := ""
	for _, rule := range ruleSet.Deny {
		matched, valid := matchArgConstraintRule(rule, arguments)
		if !valid {
			return invalidArgConstraintRuleViolation(rule)
		}
		if !matched {
			continue
		}
		denyFailures = append(denyFailures, argConstraintFailure{
			Path:       rule.Path,
			Op:         rule.Op,
			ReasonCode: argConstraintReasonDeny,
			RuleID:     rule.ID,
		})
		if firstDenyMessage == "" && strings.TrimSpace(rule.Message) != "" {
			firstDenyMessage = strings.TrimSpace(rule.Message)
		}
	}
	if len(denyFailures) > 0 {
		message := argConstraintDefaultDenyMessage
		if firstDenyMessage != "" {
			message = firstDenyMessage
		}
		return &argConstraintViolation{
			ReasonCode: argConstraintReasonDeny,
			Message:    message,
			Failures:   denyFailures,
		}
	}

	groupedAllowRules, groupOrder := groupAllowConstraintRules(ruleSet.Allow)
	allowFailures := make([]argConstraintFailure, 0)
	firstAllowMessage := ""
	for _, groupKey := range groupOrder {
		rules := groupedAllowRules[groupKey]
		matched := false
		for _, rule := range rules {
			ruleMatched, valid := matchArgConstraintRule(rule, arguments)
			if !valid {
				return invalidArgConstraintRuleViolation(rule)
			}
			if ruleMatched {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		rule := rules[0]
		allowFailures = append(allowFailures, argConstraintFailure{
			Path:       rule.Path,
			Op:         rule.Op,
			ReasonCode: argConstraintReasonNotAllowed,
			RuleID:     rule.ID,
		})
		if firstAllowMessage == "" && strings.TrimSpace(rule.Message) != "" {
			firstAllowMessage = strings.TrimSpace(rule.Message)
		}
	}

	if len(allowFailures) == 0 {
		return nil
	}
	message := argConstraintDefaultNotAllowedMessage
	if firstAllowMessage != "" {
		message = firstAllowMessage
	}
	return &argConstraintViolation{
		ReasonCode: argConstraintReasonNotAllowed,
		Message:    message,
		Failures:   allowFailures,
	}
}

func parseArgConstraintRuleSet(constraints map[string]any) (argConstraintRuleSet, bool) {
	if constraints == nil {
		return argConstraintRuleSet{}, true
	}

	rawArgs, ok := constraints["args"]
	if !ok {
		return argConstraintRuleSet{}, true
	}
	argsMap, ok := toStringAnyMap(rawArgs)
	if !ok {
		return argConstraintRuleSet{}, false
	}

	allowRules, ok := parseArgConstraintRuleList(argsMap["allow"])
	if !ok {
		return argConstraintRuleSet{}, false
	}
	denyRules, ok := parseArgConstraintRuleList(argsMap["deny"])
	if !ok {
		return argConstraintRuleSet{}, false
	}

	return argConstraintRuleSet{
		Allow: allowRules,
		Deny:  denyRules,
	}, true
}

func parseArgConstraintRuleList(value any) ([]argConstraintRule, bool) {
	if value == nil {
		return nil, true
	}
	items, ok := toAnySlice(value)
	if !ok {
		return nil, false
	}

	rules := make([]argConstraintRule, 0, len(items))
	for _, item := range items {
		ruleMap, ok := toStringAnyMap(item)
		if !ok {
			return nil, false
		}
		rule, ok := parseArgConstraintRule(ruleMap)
		if !ok {
			return nil, false
		}
		rules = append(rules, rule)
	}
	return rules, true
}

func parseArgConstraintRule(raw map[string]any) (argConstraintRule, bool) {
	path, ok := parseArgConstraintPath(raw["path"])
	if !ok {
		return argConstraintRule{}, false
	}
	op, ok := parseArgConstraintOp(raw["op"])
	if !ok {
		return argConstraintRule{}, false
	}
	value, ok := raw["value"]
	if !ok {
		return argConstraintRule{}, false
	}

	rule := argConstraintRule{
		Path:  path,
		Op:    op,
		Value: value,
	}
	if id := parseStringField(raw, "id"); id != "" {
		rule.ID = id
	} else if ruleID := parseStringField(raw, "rule_id"); ruleID != "" {
		rule.ID = ruleID
	}
	rule.Message = parseStringField(raw, "message")
	return rule, true
}

func parseArgConstraintPath(value any) (string, bool) {
	path, ok := value.(string)
	if !ok {
		return "", false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	if path == "/" {
		return path, true
	}
	if !strings.HasPrefix(path, "/") {
		return "", false
	}
	return path, true
}

func parseArgConstraintOp(value any) (string, bool) {
	op, ok := value.(string)
	if !ok {
		return "", false
	}
	op = strings.ToLower(strings.TrimSpace(op))
	switch op {
	case argConstraintOpEq, argConstraintOpPrefix, argConstraintOpRegex, argConstraintOpIn, argConstraintOpContains, argConstraintOpJSONSchema:
		return op, true
	default:
		return "", false
	}
}

func parseStringField(m map[string]any, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func groupAllowConstraintRules(rules []argConstraintRule) (map[string][]argConstraintRule, []string) {
	grouped := make(map[string][]argConstraintRule)
	order := make([]string, 0, len(rules))
	for _, rule := range rules {
		key := rule.Path + "\x00" + rule.Op
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], rule)
	}
	return grouped, order
}

func matchArgConstraintRule(rule argConstraintRule, arguments any) (bool, bool) {
	actual, ok := resolveArgConstraintPath(arguments, rule.Path)
	if !ok {
		return false, true
	}

	switch rule.Op {
	case argConstraintOpEq:
		return reflect.DeepEqual(actual, rule.Value), true
	case argConstraintOpPrefix:
		expected, ok := rule.Value.(string)
		if !ok {
			return false, false
		}
		actualString, ok := actual.(string)
		if !ok {
			return false, true
		}
		return strings.HasPrefix(actualString, expected), true
	case argConstraintOpRegex:
		pattern, ok := rule.Value.(string)
		if !ok {
			return false, false
		}
		expr, err := regexp.Compile(pattern)
		if err != nil {
			return false, false
		}
		actualString, ok := actual.(string)
		if !ok {
			return false, true
		}
		return expr.MatchString(actualString), true
	case argConstraintOpIn:
		values, ok := toAnySlice(rule.Value)
		if !ok {
			return false, false
		}
		for _, candidate := range values {
			if reflect.DeepEqual(actual, candidate) {
				return true, true
			}
		}
		return false, true
	case argConstraintOpContains:
		if actualString, ok := actual.(string); ok {
			expected, ok := rule.Value.(string)
			if !ok {
				return false, false
			}
			return strings.Contains(actualString, expected), true
		}
		values, ok := toAnySlice(actual)
		if !ok {
			return false, true
		}
		for _, item := range values {
			if reflect.DeepEqual(item, rule.Value) {
				return true, true
			}
		}
		return false, true
	case argConstraintOpJSONSchema:
		return validateArgConstraintJSONSchema(rule.Value, actual)
	default:
		return false, false
	}
}

func validateArgConstraintJSONSchema(schema, value any) (bool, bool) {
	return matchJSONSchema(schema, value)
}

func matchJSONSchema(schema, value any) (bool, bool) {
	switch typed := schema.(type) {
	case bool:
		return typed, true
	}

	schemaMap, ok := toStringAnyMap(schema)
	if !ok {
		return false, false
	}

	if enumValues, hasEnum := schemaMap["enum"]; hasEnum {
		values, ok := toAnySlice(enumValues)
		if !ok {
			return false, false
		}
		matched := false
		for _, candidate := range values {
			if reflect.DeepEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return false, true
		}
	}

	if constValue, hasConst := schemaMap["const"]; hasConst {
		if !reflect.DeepEqual(value, constValue) {
			return false, true
		}
	}

	if typeSpec, hasType := schemaMap["type"]; hasType {
		if !matchesJSONSchemaType(value, typeSpec) {
			return false, true
		}
	}

	if patternValue, hasPattern := schemaMap["pattern"]; hasPattern {
		pattern, ok := patternValue.(string)
		if !ok {
			return false, false
		}
		text, ok := value.(string)
		if !ok {
			return false, true
		}
		expr, err := regexp.Compile(pattern)
		if err != nil {
			return false, false
		}
		if !expr.MatchString(text) {
			return false, true
		}
	}

	if requiredValue, hasRequired := schemaMap["required"]; hasRequired {
		requiredKeys, ok := parseSchemaRequiredFields(requiredValue)
		if !ok {
			return false, false
		}
		obj, ok := toStringAnyMap(value)
		if !ok {
			return false, true
		}
		for _, key := range requiredKeys {
			if _, exists := obj[key]; !exists {
				return false, true
			}
		}
	}

	if propertiesValue, hasProperties := schemaMap["properties"]; hasProperties {
		properties, ok := toStringAnyMap(propertiesValue)
		if !ok {
			return false, false
		}
		obj, ok := toStringAnyMap(value)
		if !ok {
			return false, true
		}
		for key, propertySchema := range properties {
			propertyValue, exists := obj[key]
			if !exists {
				continue
			}
			matched, valid := matchJSONSchema(propertySchema, propertyValue)
			if !valid {
				return false, false
			}
			if !matched {
				return false, true
			}
		}
	}

	if itemsValue, hasItems := schemaMap["items"]; hasItems {
		items, ok := toAnySlice(value)
		if !ok {
			return false, true
		}
		for _, item := range items {
			matched, valid := matchJSONSchema(itemsValue, item)
			if !valid {
				return false, false
			}
			if !matched {
				return false, true
			}
		}
	}

	return true, true
}

func parseSchemaRequiredFields(value any) ([]string, bool) {
	items, ok := toAnySlice(value)
	if !ok {
		return nil, false
	}
	fields := make([]string, 0, len(items))
	for _, item := range items {
		field, ok := item.(string)
		if !ok {
			return nil, false
		}
		fields = append(fields, field)
	}
	return fields, true
}

func matchesJSONSchemaType(value any, typeSpec any) bool {
	var expectedTypes []string
	switch typed := typeSpec.(type) {
	case string:
		expectedTypes = []string{typed}
	default:
		items, ok := toAnySlice(typeSpec)
		if !ok {
			return false
		}
		expectedTypes = make([]string, 0, len(items))
		for _, item := range items {
			typeName, ok := item.(string)
			if !ok {
				return false
			}
			expectedTypes = append(expectedTypes, typeName)
		}
	}

	for _, typeName := range expectedTypes {
		if matchesSingleJSONSchemaType(value, typeName) {
			return true
		}
	}
	return false
}

func matchesSingleJSONSchemaType(value any, typeName string) bool {
	switch typeName {
	case "object":
		_, ok := toStringAnyMap(value)
		return ok
	case "array":
		_, ok := toAnySlice(value)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		return isJSONNumber(value)
	case "integer":
		return isJSONInteger(value)
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func isJSONNumber(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	case json.Number:
		_, err := typed.Float64()
		return err == nil
	default:
		return false
	}
}

func isJSONInteger(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return float32(math.Trunc(float64(typed))) == typed
	case float64:
		return math.Trunc(typed) == typed
	case json.Number:
		_, err := typed.Int64()
		return err == nil
	default:
		return false
	}
}

func resolveArgConstraintPath(root any, path string) (any, bool) {
	if path == "/" {
		return root, true
	}
	if !strings.HasPrefix(path, "/") {
		return nil, false
	}

	current := root
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, token := range segments {
		segment := decodeArgConstraintToken(token)

		if next, ok := getMapValue(current, segment); ok {
			current = next
			continue
		}

		values, ok := toAnySlice(current)
		if !ok {
			return nil, false
		}

		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(values) {
			return nil, false
		}
		current = values[index]
	}
	return current, true
}

func decodeArgConstraintToken(value string) string {
	value = strings.ReplaceAll(value, "~1", "/")
	return strings.ReplaceAll(value, "~0", "~")
}

func getMapValue(container any, key string) (any, bool) {
	m, ok := toStringAnyMap(container)
	if !ok {
		return nil, false
	}
	value, exists := m[key]
	return value, exists
}

func toStringAnyMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		out[iter.Key().String()] = iter.Value().Interface()
	}
	return out, true
}

func toAnySlice(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.([]any); ok {
		return typed, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func invalidArgConstraintRuleViolation(rule argConstraintRule) *argConstraintViolation {
	failures := []argConstraintFailure{{
		Path:       rule.Path,
		Op:         rule.Op,
		ReasonCode: argConstraintReasonInvalid,
		RuleID:     rule.ID,
	}}
	return &argConstraintViolation{
		ReasonCode: argConstraintReasonInvalid,
		Message:    argConstraintDefaultInvalidMessage,
		Failures:   failures,
	}
}

func argConstraintRuleID(violation *argConstraintViolation) string {
	if violation == nil {
		return "arg_constraint"
	}
	for _, failure := range violation.Failures {
		if strings.TrimSpace(failure.RuleID) != "" {
			return strings.TrimSpace(failure.RuleID)
		}
	}
	switch violation.ReasonCode {
	case argConstraintReasonDeny:
		return "arg_constraint_deny"
	case argConstraintReasonNotAllowed:
		return "arg_constraint_not_allowed"
	case argConstraintReasonInvalid:
		return "arg_constraint_invalid"
	default:
		return "arg_constraint"
	}
}

func withArgConstraintFailures(constraints map[string]any, violation *argConstraintViolation) map[string]any {
	cloned := make(map[string]any, len(constraints)+1)
	for key, value := range constraints {
		cloned[key] = value
	}
	if violation == nil || len(violation.Failures) == 0 {
		return cloned
	}
	cloned["arg_constraint_failures"] = argConstraintFailuresAsMaps(violation.Failures)
	return cloned
}

func argConstraintFailuresAsMaps(failures []argConstraintFailure) []map[string]any {
	out := make([]map[string]any, 0, len(failures))
	for _, failure := range failures {
		item := map[string]any{
			"path":        failure.Path,
			"op":          failure.Op,
			"reason_code": failure.ReasonCode,
		}
		if strings.TrimSpace(failure.RuleID) != "" {
			item["rule_id"] = strings.TrimSpace(failure.RuleID)
		}
		out = append(out, item)
	}
	return out
}

func parseRESTArguments(body []byte) any {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return trimmed
	}
	return parsed
}
