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

type ActionType string

const (
	ActionTypeCRMDelete        ActionType = "CRM.DELETE"
	ActionTypeDataDelete       ActionType = "DATA.DELETE"
	ActionTypeDataBulkExport   ActionType = "DATA.BULK_EXPORT"
	ActionTypeDataExport       ActionType = "DATA.EXPORT"
	ActionTypeAccessGrant      ActionType = "ACCESS.GRANT"
	ActionTypeCRMRead          ActionType = "CRM.READ"
	ActionTypeDataQuery        ActionType = "DATA.QUERY"
	ActionTypeDataUpdate       ActionType = "DATA.UPDATE"
	ActionTypeDataRead         ActionType = "DATA.READ"
	ActionTypeTicketCreate     ActionType = "TICKET.CREATE"
	ActionTypeTicketComment    ActionType = "TICKET.COMMENT"
	ActionTypeTicketUpdate     ActionType = "TICKET.UPDATE"
	ActionTypePaymentRefund    ActionType = "PAYMENT.REFUND"
	ActionTypeAccessRoleChange ActionType = "ACCESS.ROLE_CHANGE"
)

const (
	methodGet     = "GET"
	methodHead    = "HEAD"
	methodOptions = "OPTIONS"
	methodPost    = "POST"
	methodPut     = "PUT"
	methodPatch   = "PATCH"
	methodDelete  = "DELETE"
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
		return classifyJira(&input)
	}
	if toolID == "mock_internal" {
		return classifyMockInternal(&input)
	}
	return classifyGeneric(&input)
}

func newClassifyInput(method, path, query string) classifyInput {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = methodGet
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

func newResult(actionType ActionType, summary string) Result {
	return Result{
		ActionType:    string(actionType),
		ActionRisk:    defaultRisk(actionType),
		ActionSummary: summary,
	}
}

func classifyGeneric(input *classifyInput) Result {
	actionType := classifyGenericActionType(input)
	return newResult(actionType, requestSummary(input))
}

func classifyGenericActionType(input *classifyInput) ActionType {
	if isDeleteIntent(input) {
		if isCRMEntity(input) {
			return ActionTypeCRMDelete
		}
		return ActionTypeDataDelete
	}
	if isBulkExportIntent(input) {
		return ActionTypeDataBulkExport
	}
	if isExportIntent(input) {
		return ActionTypeDataExport
	}
	if isAccessGrantIntent(input) {
		return ActionTypeAccessGrant
	}
	if isCRMEntity(input) && (isReadMethod(input.method) || isQueryIntent(input)) {
		return ActionTypeCRMRead
	}
	if isQueryIntent(input) {
		return ActionTypeDataQuery
	}

	switch input.method {
	case methodPost, methodPut, methodPatch:
		return ActionTypeDataUpdate
	case methodDelete:
		return ActionTypeDataDelete
	default:
		return ActionTypeDataRead
	}
}

func classifyJira(input *classifyInput) Result {
	if isJiraAgilePath(input) {
		return classifyJiraAgile(input)
	}
	if isJiraIssueCreate(input) {
		return newResult(ActionTypeTicketCreate, "Create Jira issue")
	}
	if isJiraIssueComment(input) {
		return newResult(ActionTypeTicketComment, "Comment on Jira issue")
	}
	if isJiraCommentDelete(input) {
		return newResult(ActionTypeDataDelete, "Delete Jira comment")
	}
	if isJiraWatcherOrVoteRemove(input) {
		return newResult(ActionTypeTicketUpdate, "Update Jira issue")
	}
	if isJiraIssueDelete(input) {
		return newResult(ActionTypeDataDelete, "Delete Jira issue")
	}
	if isJiraIssueUpdate(input) {
		return newResult(ActionTypeTicketUpdate, "Update Jira issue")
	}
	if isJiraIssueLink(input) {
		return newResult(ActionTypeTicketUpdate, "Link Jira issues")
	}
	if isJiraSearch(input) {
		return newResult(ActionTypeDataQuery, "Query Jira issues")
	}
	return classifyGeneric(input)
}

func classifyMockInternal(input *classifyInput) Result {
	if isMockRefund(input) {
		return newResult(ActionTypePaymentRefund, "Refund payment")
	}
	if isMockExport(input) {
		return newResult(ActionTypeDataExport, "Export customer data")
	}
	if isMockRoleChange(input) {
		return newResult(ActionTypeAccessRoleChange, "Change user role")
	}
	if isMockAccessGrant(input) {
		return newResult(ActionTypeAccessGrant, "Grant access")
	}
	return classifyGeneric(input)
}

func requestSummary(input *classifyInput) string {
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
	return make(map[string]struct{}, 8) //nolint:mnd // ignore the fixed map len.
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
	case methodPost, methodPut, methodPatch, methodDelete:
		return true
	default:
		return false
	}
}

func isReadMethod(method string) bool {
	switch method {
	case methodGet, methodHead, methodOptions:
		return true
	default:
		return false
	}
}

func isDeleteIntent(input *classifyInput) bool {
	if input.method == methodDelete {
		return true
	}
	if !isWriteMethod(input.method) {
		return false
	}
	return hasAny(input.pathTokens, "delete", "remove", "revoke", "purge", "destroy", "erase")
}

func isExportIntent(input *classifyInput) bool {
	if hasAny(input.pathTokens, "export", "exports", "download", "dump", "extract", "backup") {
		return true
	}
	if hasAny(input.queryTokens, "csv", "xlsx", "jsonl", "parquet", "export", "download") {
		return true
	}
	return false
}

func isBulkExportIntent(input *classifyInput) bool {
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

func isAccessGrantIntent(input *classifyInput) bool {
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

func isQueryIntent(input *classifyInput) bool {
	if hasAny(input.pathTokens, "search", "query", "filter", "lookup", "find") {
		return true
	}
	if hasAny(input.queryTokens, "q", "query", "search", "jql", "filter", "where") {
		return true
	}
	return false
}

func isCRMEntity(input *classifyInput) bool {
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

//nolint:mnd // ignore this.
func isJiraPath(input *classifyInput) bool {
	if len(input.segments) < 4 {
		return false
	}
	return input.segments[0] == "rest" && input.segments[1] == "api"
}

func isJiraIssueCreate(input *classifyInput) bool {
	if input.method != methodPost || !isJiraPath(input) {
		return false
	}
	if len(input.segments) == 4 && input.segments[3] == "issue" {
		return true
	}
	return len(input.segments) == 5 && input.segments[3] == "issue" && input.segments[4] == "bulk"
}

func isJiraIssueComment(input *classifyInput) bool {
	if (input.method != methodPost && input.method != methodPut && input.method != methodPatch) || !isJiraPath(input) {
		return false
	}
	return hasAny(input.pathTokens, "issue") && hasAny(input.pathTokens, "comment")
}

func isJiraIssueUpdate(input *classifyInput) bool {
	if !isJiraPath(input) || !hasAny(input.pathTokens, "issue") {
		return false
	}
	if input.method == methodPut || input.method == methodPatch {
		return true
	}
	if input.method != methodPost {
		return false
	}
	return hasAny(input.pathTokens, "transition", "transitions", "assignee", "watcher", "watchers", "worklog", "attachment", "attachments", "archive", "unarchive")
}

func isJiraCommentDelete(input *classifyInput) bool {
	return input.method == methodDelete &&
		isJiraPath(input) &&
		hasAny(input.pathTokens, "issue") &&
		hasAny(input.pathTokens, "comment")
}

func isJiraWatcherOrVoteRemove(input *classifyInput) bool {
	return input.method == methodDelete &&
		isJiraPath(input) &&
		hasAny(input.pathTokens, "issue") &&
		hasAny(input.pathTokens, "watcher", "watchers", "vote", "votes")
}

func isJiraIssueLink(input *classifyInput) bool {
	if !isJiraPath(input) {
		return false
	}
	return hasAny(input.pathTokens, "issuelink", "remotelink")
}

func isJiraIssueDelete(input *classifyInput) bool {
	return input.method == methodDelete && isJiraPath(input) && hasAny(input.pathTokens, "issue")
}

func isJiraAgilePath(input *classifyInput) bool {
	if len(input.segments) < 3 {
		return false
	}
	return input.segments[0] == "rest" && input.segments[1] == "agile"
}

func classifyJiraAgile(input *classifyInput) Result {
	if hasAny(input.pathTokens, "sprint") && hasAny(input.pathTokens, "issue") {
		if input.method == methodPost {
			return newResult(ActionTypeTicketUpdate, "Move issues to sprint")
		}
		return newResult(ActionTypeDataQuery, "Query sprint issues")
	}
	if hasAny(input.pathTokens, "sprint") {
		switch input.method {
		case methodPost:
			return newResult(ActionTypeTicketCreate, "Create sprint")
		case methodPut, methodPatch:
			return newResult(ActionTypeTicketUpdate, "Update sprint")
		case methodDelete:
			return newResult(ActionTypeDataDelete, "Delete sprint")
		default:
			return newResult(ActionTypeDataQuery, "Query sprints")
		}
	}
	if hasAny(input.pathTokens, "board") {
		return newResult(ActionTypeDataQuery, "Query Jira board")
	}
	return classifyGeneric(input)
}

func isJiraSearch(input *classifyInput) bool {
	if !isJiraPath(input) {
		return false
	}
	if hasAny(input.pathTokens, "search", "jql") {
		return true
	}
	return hasAny(input.queryTokens, "jql", "query", "search")
}

func isMockRefund(input *classifyInput) bool {
	if input.method != methodPost && input.method != methodPut && input.method != methodPatch {
		return false
	}
	return hasAny(input.pathTokens, "refund", "refunds")
}

func isMockExport(input *classifyInput) bool {
	if input.method == methodDelete {
		return false
	}
	return hasAny(input.pathTokens, "export", "exports", "download")
}

func isMockRoleChange(input *classifyInput) bool {
	if input.method != methodPost && input.method != methodPut && input.method != methodPatch {
		return false
	}
	hasRoleToken := hasAny(input.pathTokens, "role", "roles")
	return hasRoleToken && hasAny(input.pathTokens, "change", "update", "assign", "set")
}

func isMockAccessGrant(input *classifyInput) bool {
	if input.method != methodPost && input.method != methodPut && input.method != methodPatch {
		return false
	}
	if hasAny(input.pathTokens, "grant", "invite") && hasAny(input.pathTokens, "access", "permission", "permissions", "role", "roles") {
		return true
	}
	return hasAll(input.pathTokens, "permissions", "assign")
}

func defaultRisk(actionType ActionType) string {
	switch actionType {
	case ActionTypePaymentRefund, ActionTypeAccessRoleChange:
		return RiskHigh
	case ActionTypeDataExport, ActionTypeAccessGrant, ActionTypeDataBulkExport, ActionTypeDataDelete, ActionTypeCRMDelete:
		return RiskCritical
	case ActionTypeTicketCreate, ActionTypeTicketComment, ActionTypeTicketUpdate, ActionTypeDataRead, ActionTypeDataQuery, ActionTypeCRMRead:
		return RiskLow
	default:
		return RiskMedium
	}
}
