package classification

import (
	"net/url"
	pathpkg "path"
	"strings"
)

type Result struct {
	ActionType    string
	ActionRisk    string
	ActionSummary string
}

const (
	RiskLow      = "LOW"
	RiskMedium   = "MEDIUM"
	RiskHigh     = "HIGH"
	RiskCritical = "CRITICAL"
)

type classifyInput struct {
	method         string
	rawPath        string
	normalizedPath string
	segments       []string
	pathTokens     map[string]struct{}
	queryValues    url.Values
	queryTokens    map[string]struct{}
}

func Classify(toolID, method, path, query string, headers map[string]string) Result {
	_ = headers

	input := newClassifyInput(method, path, query)
	toolID = strings.ToLower(strings.TrimSpace(toolID))

	if toolID == "jira" {
		return classifyJira(input)
	}
	if toolID == "mock_internal" {
		return classifyMockInternal(input)
	}
	return classifyGeneric(input)
}

func newClassifyInput(method, path, query string) classifyInput {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = "GET"
	}

	rawPath := strings.TrimSpace(path)
	if rawPath == "" {
		rawPath = "/"
	}

	normalizedPath := normalizePath(rawPath)
	segments := pathSegments(normalizedPath)
	pathTokens := makeTokenSet()
	addTokens(pathTokens, segments...)

	queryValues := parseQueryValues(query)
	queryTokens := makeTokenSet()
	addQueryTokens(queryTokens, queryValues)

	return classifyInput{
		method:         normalizedMethod,
		rawPath:        rawPath,
		normalizedPath: normalizedPath,
		segments:       segments,
		pathTokens:     pathTokens,
		queryValues:    queryValues,
		queryTokens:    queryTokens,
	}
}

func classifyGeneric(input classifyInput) Result {
	actionType := classifyGenericActionType(input)
	return Result{
		ActionType:    actionType,
		ActionRisk:    defaultRisk(actionType),
		ActionSummary: requestSummary(input),
	}
}

func classifyGenericActionType(input classifyInput) string {
	if isDeleteIntent(input) {
		if isCRMEntity(input) {
			return "CRM.DELETE"
		}
		return "DATA.DELETE"
	}
	if isBulkExportIntent(input) {
		return "DATA.BULK_EXPORT"
	}
	if isExportIntent(input) {
		return "DATA.EXPORT"
	}
	if isAccessGrantIntent(input) {
		return "ACCESS.GRANT"
	}
	if isCRMEntity(input) && (isReadMethod(input.method) || isQueryIntent(input)) {
		return "CRM.READ"
	}
	if isQueryIntent(input) {
		return "DATA.QUERY"
	}

	switch input.method {
	case "POST", "PUT", "PATCH":
		return "DATA.UPDATE"
	case "DELETE":
		return "DATA.DELETE"
	default:
		return "DATA.READ"
	}
}

func classifyJira(input classifyInput) Result {
	if isJiraIssueCreate(input) {
		return Result{
			ActionType:    "TICKET.CREATE",
			ActionRisk:    defaultRisk("TICKET.CREATE"),
			ActionSummary: "Create Jira issue",
		}
	}
	if isJiraIssueComment(input) {
		return Result{
			ActionType:    "TICKET.COMMENT",
			ActionRisk:    defaultRisk("TICKET.COMMENT"),
			ActionSummary: "Comment on Jira issue",
		}
	}
	if isJiraCommentDelete(input) {
		return Result{
			ActionType:    "DATA.DELETE",
			ActionRisk:    defaultRisk("DATA.DELETE"),
			ActionSummary: "Delete Jira comment",
		}
	}
	if isJiraIssueDelete(input) {
		return Result{
			ActionType:    "DATA.DELETE",
			ActionRisk:    defaultRisk("DATA.DELETE"),
			ActionSummary: "Delete Jira issue",
		}
	}
	if isJiraIssueUpdate(input) {
		return Result{
			ActionType:    "TICKET.UPDATE",
			ActionRisk:    defaultRisk("TICKET.UPDATE"),
			ActionSummary: "Update Jira issue",
		}
	}
	if isJiraSearch(input) {
		return Result{
			ActionType:    "DATA.QUERY",
			ActionRisk:    defaultRisk("DATA.QUERY"),
			ActionSummary: "Query Jira issues",
		}
	}
	return classifyGeneric(input)
}

func classifyMockInternal(input classifyInput) Result {
	if isMockRefund(input) {
		return Result{
			ActionType:    "PAYMENT.REFUND",
			ActionRisk:    defaultRisk("PAYMENT.REFUND"),
			ActionSummary: "Refund payment",
		}
	}
	if isMockExport(input) {
		return Result{
			ActionType:    "DATA.EXPORT",
			ActionRisk:    defaultRisk("DATA.EXPORT"),
			ActionSummary: "Export customer data",
		}
	}
	if isMockRoleChange(input) {
		return Result{
			ActionType:    "ACCESS.ROLE_CHANGE",
			ActionRisk:    defaultRisk("ACCESS.ROLE_CHANGE"),
			ActionSummary: "Change user role",
		}
	}
	if isMockAccessGrant(input) {
		return Result{
			ActionType:    "ACCESS.GRANT",
			ActionRisk:    defaultRisk("ACCESS.GRANT"),
			ActionSummary: "Grant access",
		}
	}
	return classifyGeneric(input)
}

func requestSummary(input classifyInput) string {
	return input.method + " " + input.rawPath
}

func normalizePath(rawPath string) string {
	cleaned := strings.TrimSpace(rawPath)
	if cleaned == "" {
		return "/"
	}

	if parsed, err := url.Parse(cleaned); err == nil {
		if parsed.Path != "" {
			cleaned = parsed.Path
		} else if left, _, ok := strings.Cut(cleaned, "?"); ok {
			cleaned = left
		}
	} else if left, _, ok := strings.Cut(cleaned, "?"); ok {
		cleaned = left
	}

	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	normalized := strings.ToLower(pathpkg.Clean(cleaned))
	if normalized == "." {
		return "/"
	}
	return normalized
}

func parseQueryValues(rawQuery string) url.Values {
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return nil
	}
	query = strings.TrimPrefix(query, "?")
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil
	}
	return values
}

func pathSegments(normalizedPath string) []string {
	trimmed := strings.Trim(normalizedPath, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func makeTokenSet() map[string]struct{} {
	return make(map[string]struct{}, 8)
}

func addTokens(set map[string]struct{}, values ...string) {
	for _, value := range values {
		for _, token := range tokenize(value) {
			if token == "" {
				continue
			}
			set[token] = struct{}{}
		}
	}
}

func addQueryTokens(set map[string]struct{}, values url.Values) {
	for key, entries := range values {
		addTokens(set, key)
		for _, entry := range entries {
			addTokens(set, entry)
		}
	}
}

func tokenize(value string) []string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return nil
	}
	replacer := strings.NewReplacer(
		"-", " ",
		"_", " ",
		".", " ",
		":", " ",
		",", " ",
		";", " ",
		"=", " ",
		"+", " ",
		"|", " ",
	)
	return strings.Fields(replacer.Replace(normalized))
}

func hasAny(set map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func hasAll(set map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func isWriteMethod(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func isReadMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func isDeleteIntent(input classifyInput) bool {
	if input.method == "DELETE" {
		return true
	}
	if !isWriteMethod(input.method) {
		return false
	}
	return hasAny(input.pathTokens, "delete", "remove", "revoke", "purge", "destroy", "erase")
}

func isExportIntent(input classifyInput) bool {
	if hasAny(input.pathTokens, "export", "exports", "download", "dump", "extract", "backup") {
		return true
	}
	if hasAny(input.queryTokens, "csv", "xlsx", "jsonl", "parquet", "export", "download") {
		return true
	}
	return false
}

func isBulkExportIntent(input classifyInput) bool {
	if !isExportIntent(input) {
		return false
	}
	if hasAny(input.pathTokens, "bulk", "batch", "mass", "multi", "all") {
		return true
	}
	if hasAny(input.queryTokens, "bulk", "batch", "all", "include", "includeall") {
		return true
	}
	return false
}

func isAccessGrantIntent(input classifyInput) bool {
	if !isWriteMethod(input.method) {
		return false
	}
	hasRoleToken := hasAny(input.pathTokens, "role", "roles")
	if hasRoleToken && hasAny(input.pathTokens, "change", "assign", "update") {
		return true
	}
	if hasAny(input.pathTokens, "grant", "permission", "permissions", "invite", "membership", "member", "members") {
		return true
	}
	return false
}

func isQueryIntent(input classifyInput) bool {
	if hasAny(input.pathTokens, "search", "query", "filter", "lookup", "find") {
		return true
	}
	if hasAny(input.queryTokens, "q", "query", "search", "jql", "filter", "where") {
		return true
	}
	return false
}

func isCRMEntity(input classifyInput) bool {
	return hasAny(
		input.pathTokens,
		"crm",
		"contact",
		"contacts",
		"lead",
		"leads",
		"account",
		"accounts",
		"opportunity",
		"opportunities",
		"deal",
		"deals",
		"customer",
		"customers",
	)
}

func isJiraPath(input classifyInput) bool {
	if len(input.segments) < 4 {
		return false
	}
	return input.segments[0] == "rest" && input.segments[1] == "api"
}

func isJiraIssueCreate(input classifyInput) bool {
	if input.method != "POST" || !isJiraPath(input) {
		return false
	}
	if len(input.segments) == 4 && input.segments[3] == "issue" {
		return true
	}
	return len(input.segments) == 5 && input.segments[3] == "issue" && input.segments[4] == "bulk"
}

func isJiraIssueComment(input classifyInput) bool {
	if (input.method != "POST" && input.method != "PUT" && input.method != "PATCH") || !isJiraPath(input) {
		return false
	}
	return hasAny(input.pathTokens, "issue") && hasAny(input.pathTokens, "comment")
}

func isJiraIssueUpdate(input classifyInput) bool {
	if !isJiraPath(input) || !hasAny(input.pathTokens, "issue") {
		return false
	}
	if input.method == "PUT" || input.method == "PATCH" {
		return true
	}
	if input.method != "POST" {
		return false
	}
	return hasAny(input.pathTokens, "transition", "transitions", "assignee", "watcher", "watchers", "worklog")
}

func isJiraCommentDelete(input classifyInput) bool {
	return input.method == "DELETE" &&
		isJiraPath(input) &&
		hasAny(input.pathTokens, "issue") &&
		hasAny(input.pathTokens, "comment")
}

func isJiraIssueDelete(input classifyInput) bool {
	return input.method == "DELETE" && isJiraPath(input) && hasAny(input.pathTokens, "issue")
}

func isJiraSearch(input classifyInput) bool {
	if !isJiraPath(input) {
		return false
	}
	if hasAny(input.pathTokens, "search", "jql") {
		return true
	}
	return hasAny(input.queryTokens, "jql", "query", "search")
}

func isMockRefund(input classifyInput) bool {
	if input.method != "POST" && input.method != "PUT" && input.method != "PATCH" {
		return false
	}
	return hasAny(input.pathTokens, "refund", "refunds")
}

func isMockExport(input classifyInput) bool {
	if input.method == "DELETE" {
		return false
	}
	return hasAny(input.pathTokens, "export", "exports", "download")
}

func isMockRoleChange(input classifyInput) bool {
	if input.method != "POST" && input.method != "PUT" && input.method != "PATCH" {
		return false
	}
	hasRoleToken := hasAny(input.pathTokens, "role", "roles")
	return hasRoleToken && hasAny(input.pathTokens, "change", "update", "assign", "set")
}

func isMockAccessGrant(input classifyInput) bool {
	if input.method != "POST" && input.method != "PUT" && input.method != "PATCH" {
		return false
	}
	if hasAny(input.pathTokens, "grant", "invite") && hasAny(input.pathTokens, "access", "permission", "permissions", "role", "roles") {
		return true
	}
	return hasAll(input.pathTokens, "permissions", "assign")
}

func defaultRisk(actionType string) string {
	switch actionType {
	case "PAYMENT.REFUND", "ACCESS.ROLE_CHANGE":
		return RiskHigh
	case "DATA.EXPORT", "ACCESS.GRANT", "DATA.BULK_EXPORT", "DATA.DELETE", "CRM.DELETE":
		return RiskCritical
	case "TICKET.CREATE", "TICKET.COMMENT", "TICKET.UPDATE", "DATA.READ", "DATA.QUERY", "CRM.READ":
		return RiskLow
	default:
		return RiskMedium
	}
}
