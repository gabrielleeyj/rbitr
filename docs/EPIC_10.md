# EPIC 10 — Enterprise Integrations

## Summary

Epic 10 adds governed integrations with enterprise tools, making rbitr the control plane for AI agents operating across the enterprise SaaS stack.

The integration model is consistent with the existing Jira pattern: each integration registers a `toolID` in the classifier, maps its endpoints to the standard action taxonomy (`TICKET.*`, `CRM.*`, `DATA.*`, `ACCESS.*`, `NOTIFICATION.*`), and exposes a seed migration for demo/dev tool wiring. No new connector protocol is needed for most integrations — they reuse the existing generic REST connector.

Date scoped: 2026-03-06

---

## Part 1 — Jira Integration Audit

### Current Implementation

The Jira classifier (`internal/classification/classifier.go`) covers:

| HTTP Method | Path Pattern | Classified As |
|---|---|---|
| `POST` | `/rest/api/{v}/issue` | `TICKET.CREATE` |
| `POST` | `/rest/api/{v}/issue/bulk` | `TICKET.CREATE` |
| `POST` | `/rest/api/{v}/issue/{key}/comment` | `TICKET.COMMENT` |
| `PUT` / `PATCH` | `/rest/api/{v}/issue/{key}/comment/{id}` | `TICKET.COMMENT` |
| `DELETE` | `/rest/api/{v}/issue/{key}/comment/{id}` | `DATA.DELETE` |
| `PUT` / `PATCH` | `/rest/api/{v}/issue/{key}` | `TICKET.UPDATE` |
| `POST` | `/rest/api/{v}/issue/{key}/transitions` | `TICKET.UPDATE` |
| `POST` | `/rest/api/{v}/issue/{key}/assignee` | `TICKET.UPDATE` |
| `POST` | `/rest/api/{v}/issue/{key}/watchers` | `TICKET.UPDATE` |
| `POST` | `/rest/api/{v}/issue/{key}/worklog` | `TICKET.UPDATE` |
| `DELETE` | `/rest/api/{v}/issue/{key}` | `DATA.DELETE` |
| `GET` | `/rest/api/{v}/search?jql=...` | `DATA.QUERY` |
| `POST` | `/rest/api/{v}/search/jql` | `DATA.QUERY` |

The `isJiraPath` guard correctly validates paths starting with `/rest/api/` and requiring at least 4 segments, making it version-agnostic (works for both v2 and v3).

### Confirmed Correct Against Jira Cloud REST API v3

All endpoints above map accurately to the Atlassian Jira Cloud REST API v3 spec:

- `POST /rest/api/3/issue` — Create issue (**correct**)
- `POST /rest/api/3/issue/bulk` — Bulk create issues (**correct**)
- `PUT /rest/api/3/issue/{issueIdOrKey}` — Edit issue (**correct**)
- `DELETE /rest/api/3/issue/{issueIdOrKey}` — Delete issue (**correct**)
- `POST /rest/api/3/issue/{issueIdOrKey}/comment` — Add comment (**correct**)
- `PUT /rest/api/3/issue/{issueIdOrKey}/comment/{id}` — Update comment (**correct**)
- `DELETE /rest/api/3/issue/{issueIdOrKey}/comment/{id}` — Delete comment (**correct**)
- `POST /rest/api/3/issue/{issueIdOrKey}/transitions` — Transition issue (**correct**)
- `POST /rest/api/3/issue/{issueIdOrKey}/watchers` — Add watcher (**correct**)
- `POST /rest/api/3/issue/{issueIdOrKey}/worklog` — Add worklog (**correct**)
- `GET /rest/api/3/search` — Search via JQL (**correct**)
- `POST /rest/api/3/search/jql` — Search via JQL body (**correct**, introduced in newer Cloud API versions)

### Gaps Identified

The following Jira Cloud REST API v3 endpoints are not explicitly classified and fall through to `classifyGeneric`:

**1. Attachments**

`POST /rest/api/3/issue/{issueIdOrKey}/attachments` is classified as `DATA.UPDATE` by the generic classifier (POST method fallback). This is semantically reasonable but not explicit. Consider adding a dedicated `TICKET.ATTACH` action type or explicitly routing it to `TICKET.UPDATE`.

**2. Issue Links**

`POST /rest/api/3/issueLink` — the path tokenizes to `[rest, api, 3, issuelink]`. The token `issue` is NOT present (the tokenizer does not split camelCase), so `hasAny(pathTokens, "issue")` returns false. This falls to `classifyGeneric` → `DATA.UPDATE`. Should be classified as `TICKET.UPDATE`.

`DELETE /rest/api/3/issueLink/{linkId}` similarly falls to `DATA.DELETE` via generic classifier. Risk is acceptable but the action summary is generic.

**3. Jira Agile / Jira Software Sprint Endpoints**

`/rest/agile/1.0/sprint/{sprintId}/issue` has `segments[1] == "agile"` rather than `"api"`, so `isJiraPath` returns false. These endpoints (sprint issue moves, board queries) fall entirely to the generic classifier.

Relevant Jira Agile endpoints missed:
- `POST /rest/agile/1.0/sprint/{sprintId}/issue` — move issues to sprint → should be `TICKET.UPDATE`
- `GET /rest/agile/1.0/board/{boardId}/sprint` — query sprints → should be `DATA.QUERY`
- `GET /rest/agile/1.0/board` — list boards → should be `DATA.READ`

**4. Watcher Removal**

`DELETE /rest/api/3/issue/{issueIdOrKey}/watchers` is classified as `DATA.DELETE` via `isJiraIssueDelete` (has "issue" token, is DELETE). The risk tier (`CRITICAL`) is overstated for a watcher removal. Should be `TICKET.UPDATE` with `LOW` risk.

**5. Archive**

`POST /rest/api/3/issue/{issueIdOrKey}/archive` falls to `classifyGeneric`. The path has "archive" which triggers `isExportIntent`, classifying it as `DATA.EXPORT` (risk: `CRITICAL`). This is incorrect — archiving an issue is closer to `TICKET.UPDATE` (`MEDIUM` risk).

### Recommendations for Epic 10

- Add `isJiraAgile` classifier that checks `segments[1] == "agile"` as the Jira Agile path guard.
- Fix issue link classification by matching `issuelink` as a path token in addition to `issue`.
- Add explicit handling for attachment endpoints.
- Fix watcher removal to map to `TICKET.UPDATE` before the generic delete check fires.
- Add archive/unarchive to `TICKET.UPDATE` classification.

These are low-effort fixes with clear test cases. They should be bundled as a sub-story in this epic.

---

## Part 2 — Enterprise Integration Roadmap

### Ranking Criteria

Each integration is ranked easiest-to-hardest based on:

1. **Auth complexity** — how different is it from existing `bearer` / `api_key` patterns?
2. **Endpoint predictability** — are paths regular REST, or do they require special routing logic (GraphQL, RPC, dynamic base URLs)?
3. **Action taxonomy fit** — do the endpoints map cleanly to existing action types?
4. **Scope relevance** — how much of the API surface needs classification for meaningful governance?

---

### Integration 1 — Confluence (Rank: Easiest)

**Why easiest:** Shares Atlassian auth with Jira (same Bearer token / API token). REST API path structure is similar to Jira (`/wiki/rest/api/v1/...` or `/rest/api/...`). Many tenants deploying Jira also use Confluence.

**Atlassian Confluence Cloud REST API:**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/wiki/rest/api/content` | `DATA.QUERY` | `LOW` |
| `GET` | `/wiki/rest/api/content/{id}` | `DATA.READ` | `LOW` |
| `POST` | `/wiki/rest/api/content` | `TICKET.CREATE` | `LOW` |
| `PUT` | `/wiki/rest/api/content/{id}` | `TICKET.UPDATE` | `LOW` |
| `DELETE` | `/wiki/rest/api/content/{id}` | `DATA.DELETE` | `CRITICAL` |
| `POST` | `/wiki/rest/api/content/{id}/child/comment` | `TICKET.COMMENT` | `LOW` |
| `GET` | `/wiki/rest/api/search` | `DATA.QUERY` | `LOW` |
| `POST` | `/wiki/rest/api/content/{id}/restriction` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/wiki/rest/api/content/{id}/restriction` | `ACCESS.GRANT` | `CRITICAL` |

**Auth type:** `bearer` (same as Jira — API token or user PAT). No new connector logic needed.

**New action types required:** None. Reuses existing taxonomy.

**Classifier guard:** `isConfluencePath` checks `segments[0] == "wiki" && segments[1] == "rest"` or `segments[0] == "rest" && segments[2] == "content"`.

**Implementation effort:** ~1 day classifier + tests + seed migration.

---

### Integration 2 — GitHub (Rank: Easy)

**Why easy:** Simple Bearer token (Personal Access Token) auth. GitHub REST API is well-documented, versioned, and follows consistent path conventions. Issue/PR lifecycle maps directly to `TICKET.*` action types.

**GitHub REST API v3 (`api.github.com`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/repos/{owner}/{repo}/issues` | `DATA.QUERY` | `LOW` |
| `POST` | `/repos/{owner}/{repo}/issues` | `TICKET.CREATE` | `LOW` |
| `PATCH` | `/repos/{owner}/{repo}/issues/{number}` | `TICKET.UPDATE` | `LOW` |
| `POST` | `/repos/{owner}/{repo}/issues/{number}/comments` | `TICKET.COMMENT` | `LOW` |
| `PATCH` | `/repos/{owner}/{repo}/issues/{number}/comments/{id}` | `TICKET.COMMENT` | `LOW` |
| `DELETE` | `/repos/{owner}/{repo}/issues/{number}/comments/{id}` | `DATA.DELETE` | `CRITICAL` |
| `POST` | `/repos/{owner}/{repo}/issues/{number}/assignees` | `TICKET.UPDATE` | `LOW` |
| `POST` | `/repos/{owner}/{repo}/issues/{number}/labels` | `TICKET.UPDATE` | `LOW` |
| `GET` | `/repos/{owner}/{repo}/pulls` | `DATA.QUERY` | `LOW` |
| `POST` | `/repos/{owner}/{repo}/pulls` | `TICKET.CREATE` | `LOW` |
| `PUT` | `/repos/{owner}/{repo}/pulls/{number}/merge` | `TICKET.UPDATE` | `HIGH` |
| `POST` | `/repos/{owner}/{repo}/pulls/{number}/reviews` | `TICKET.COMMENT` | `LOW` |
| `POST` | `/repos/{owner}/{repo}/collaborators/{username}` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/repos/{owner}/{repo}/collaborators/{username}` | `ACCESS.GRANT` | `CRITICAL` |
| `GET` | `/search/issues` | `DATA.QUERY` | `LOW` |
| `GET` | `/search/code` | `DATA.QUERY` | `LOW` |

**Auth type:** `bearer` (Personal Access Token or GitHub App token). No new connector logic.

**New action types required:** None. PR merge could reuse `TICKET.UPDATE` with `HIGH` risk.

**Classifier guard:** `isGitHubPath` checks `segments[0] == "repos"` or `segments[0] == "search"` or `segments[0] == "orgs"`.

**Implementation effort:** ~1.5 days classifier + tests + seed migration.

**Enterprise note:** GitHub Enterprise Server uses the same API paths under a custom domain, so the classifier is reusable without modification.

---

### Integration 3 — PagerDuty (Rank: Easy-Medium)

**Why easy-medium:** Simple REST API with API key auth. Clear incident lifecycle that maps cleanly to the action taxonomy. Limited surface area for meaningful governance.

**PagerDuty REST API v2 (`api.pagerduty.com`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/incidents` | `DATA.QUERY` | `LOW` |
| `POST` | `/incidents` | `TICKET.CREATE` | `MEDIUM` |
| `GET` | `/incidents/{id}` | `DATA.READ` | `LOW` |
| `PUT` | `/incidents/{id}` | `TICKET.UPDATE` | `MEDIUM` |
| `PUT` | `/incidents` (bulk) | `TICKET.UPDATE` | `HIGH` |
| `POST` | `/incidents/{id}/notes` | `TICKET.COMMENT` | `LOW` |
| `POST` | `/incidents/{id}/snooze` | `TICKET.UPDATE` | `LOW` |
| `GET` | `/services` | `DATA.QUERY` | `LOW` |
| `POST` | `/services` | `TICKET.CREATE` | `MEDIUM` |
| `DELETE` | `/services/{id}` | `DATA.DELETE` | `CRITICAL` |
| `GET` | `/escalation_policies` | `DATA.QUERY` | `LOW` |
| `POST` | `/escalation_policies` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `PUT` | `/escalation_policies/{id}` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `POST` | `/users` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/users/{id}` | `DATA.DELETE` | `CRITICAL` |

**Auth type:** `api_key` with `Authorization: Token token={key}` header. Already supported by the generic connector with auth type `api_key`.

**New action types required:** None.

**Classifier guard:** `isPagerDutyPath` checks `segments[0]` against known resource names: `incidents`, `services`, `escalation_policies`, `users`, `teams`, `schedules`.

**Implementation effort:** ~1.5 days.

---

### Integration 4 — Opsgenie (Rank: Easy-Medium)

**Why easy-medium:** Nearly identical to PagerDuty in structure. Atlassian-owned, so familiar territory. Alert-centric with clear action taxonomy mapping.

**Opsgenie REST API (`api.opsgenie.com/v2`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/v2/alerts` | `DATA.QUERY` | `LOW` |
| `POST` | `/v2/alerts` | `TICKET.CREATE` | `MEDIUM` |
| `GET` | `/v2/alerts/{id}` | `DATA.READ` | `LOW` |
| `DELETE` | `/v2/alerts/{id}` | `DATA.DELETE` | `CRITICAL` |
| `POST` | `/v2/alerts/{id}/acknowledge` | `TICKET.UPDATE` | `LOW` |
| `POST` | `/v2/alerts/{id}/close` | `TICKET.UPDATE` | `LOW` |
| `POST` | `/v2/alerts/{id}/notes` | `TICKET.COMMENT` | `LOW` |
| `POST` | `/v2/alerts/{id}/escalate` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `GET` | `/v2/on-calls` | `DATA.QUERY` | `LOW` |
| `POST` | `/v2/users` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/v2/users/{id}` | `DATA.DELETE` | `CRITICAL` |

**Auth type:** `api_key` (`Authorization: GenieKey {key}`). Handled by generic connector.

**Implementation effort:** ~1 day (similar enough to PagerDuty that both can be built in parallel).

---

### Integration 5 — Slack (Rank: Medium)

**Why medium:** Slack Web API uses flat method names (e.g. `chat.postMessage`) rather than REST path conventions. The base URL is `https://slack.com/api/{method}` — all calls are `POST` to the same host with the method name in the path. The classifier needs a different matching strategy: method name lookup rather than path segment guards.

This requires a new `NOTIFICATION.*` action domain.

**Slack Web API (`slack.com/api`):**

| Path (method name) | Action Type | Risk |
|---|---|---|
| `chat.postMessage` | `NOTIFICATION.SEND` | `MEDIUM` |
| `chat.update` | `NOTIFICATION.UPDATE` | `LOW` |
| `chat.delete` | `DATA.DELETE` | `HIGH` |
| `chat.scheduleMessage` | `NOTIFICATION.SEND` | `MEDIUM` |
| `conversations.create` | `ACCESS.GRANT` | `HIGH` |
| `conversations.invite` | `ACCESS.GRANT` | `HIGH` |
| `conversations.kick` | `ACCESS.GRANT` | `HIGH` |
| `conversations.archive` | `DATA.DELETE` | `HIGH` |
| `conversations.history` | `DATA.QUERY` | `LOW` |
| `files.upload` | `DATA.UPDATE` | `MEDIUM` |
| `files.delete` | `DATA.DELETE` | `CRITICAL` |
| `users.admin.setOwner` | `ACCESS.ROLE_CHANGE` | `CRITICAL` |
| `usergroups.create` | `ACCESS.GRANT` | `HIGH` |
| `usergroups.users.update` | `ACCESS.ROLE_CHANGE` | `HIGH` |

**Auth type:** `bearer` (Bot token `xoxb-...`). Handled by generic connector.

**New action types required:**
- `NOTIFICATION.SEND` — send a message to a channel or user
- `NOTIFICATION.UPDATE` — edit a posted message

**Classifier strategy:** `isSlackPath` checks `segments[0] == "api"` and last segment contains a `.` (Slack method names). Match against a known method lookup table.

**Implementation effort:** ~2 days. Requires new action types and a method-lookup matching strategy rather than segment-based guard.

---

### Integration 6 — HubSpot (Rank: Medium)

**Why medium:** HubSpot CRM REST API v3 is well-structured and maps directly to existing `CRM.*` action types. Auth is simple (private app token). The challenge is the large API surface (50+ object types) — governance needs to cover the meaningful subset (contacts, companies, deals, notes, emails).

**HubSpot CRM API v3 (`api.hubapi.com`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/crm/v3/objects/{objectType}` | `CRM.READ` | `LOW` |
| `POST` | `/crm/v3/objects/{objectType}` | `DATA.UPDATE` | `MEDIUM` |
| `GET` | `/crm/v3/objects/{objectType}/{objectId}` | `CRM.READ` | `LOW` |
| `PATCH` | `/crm/v3/objects/{objectType}/{objectId}` | `DATA.UPDATE` | `MEDIUM` |
| `DELETE` | `/crm/v3/objects/{objectType}/{objectId}` | `CRM.DELETE` | `CRITICAL` |
| `POST` | `/crm/v3/objects/{objectType}/batch/read` | `CRM.READ` | `LOW` |
| `POST` | `/crm/v3/objects/{objectType}/batch/create` | `DATA.UPDATE` | `HIGH` |
| `POST` | `/crm/v3/objects/{objectType}/batch/update` | `DATA.UPDATE` | `HIGH` |
| `POST` | `/crm/v3/objects/{objectType}/batch/archive` | `CRM.DELETE` | `CRITICAL` |
| `POST` | `/crm/v3/objects/{objectType}/search` | `DATA.QUERY` | `LOW` |
| `GET` | `/crm/v3/associations/{fromObjectType}/{toObjectType}/batch/read` | `CRM.READ` | `LOW` |
| `GET` | `/marketing/v3/emails` | `DATA.QUERY` | `LOW` |
| `POST` | `/marketing/v3/emails/{emailId}/send` | `NOTIFICATION.SEND` | `HIGH` |

**Auth type:** `bearer` (Private app token). No new connector logic.

**New action types required:** `NOTIFICATION.SEND` (if Slack integration has already added it, reuse it).

**Classifier guard:** `isHubSpotPath` checks `segments[0]` against `crm`, `marketing`, `contacts`, `engagements`.

**Implementation effort:** ~2 days. Meaningful because CRM data is high-value and enterprises need CRM action governance.

---

### Integration 7 — ServiceNow (Rank: Medium-Hard)

**Why medium-hard:** ServiceNow Table API is REST-based but table-driven. The base URL is instance-specific (`{instance}.service-now.com`), which rbitr already handles via per-tool `base_url`. The challenge is the table-based routing: governance must be aware of which table an operation targets to correctly classify the risk (`incident` vs `sys_user` vs `cmdb_ci` are very different risk tiers).

**ServiceNow Table API (`{instance}.service-now.com/api/now/table`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/api/now/table/incident` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/now/table/incident` | `TICKET.CREATE` | `MEDIUM` |
| `GET` | `/api/now/table/incident/{sys_id}` | `DATA.READ` | `LOW` |
| `PATCH` | `/api/now/table/incident/{sys_id}` | `TICKET.UPDATE` | `MEDIUM` |
| `DELETE` | `/api/now/table/incident/{sys_id}` | `DATA.DELETE` | `CRITICAL` |
| `POST` | `/api/now/table/change_request` | `TICKET.CREATE` | `HIGH` |
| `PATCH` | `/api/now/table/change_request/{sys_id}` | `TICKET.UPDATE` | `HIGH` |
| `GET` | `/api/now/table/sys_user` | `DATA.QUERY` | `MEDIUM` |
| `POST` | `/api/now/table/sys_user` | `ACCESS.GRANT` | `CRITICAL` |
| `PATCH` | `/api/now/table/sys_user/{sys_id}` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `DELETE` | `/api/now/table/sys_user/{sys_id}` | `DATA.DELETE` | `CRITICAL` |
| `GET` | `/api/now/table/cmdb_ci` | `DATA.QUERY` | `MEDIUM` |
| `PATCH` | `/api/now/table/cmdb_ci/{sys_id}` | `DATA.UPDATE` | `HIGH` |
| `POST` | `/api/now/v2/table/{tableName}/bulk` | `DATA.UPDATE` | `CRITICAL` |

**Auth type:** `bearer` (OAuth 2.0 or basic auth). The basic-auth variant needs a new connector auth type or credential encoding in `auth_value`.

**Classifier strategy:** `isServiceNowPath` checks `segments[0] == "api" && segments[1] == "now"`. A second-level lookup determines risk override based on the table name in `segments[3]`:
- `incident`, `problem`, `sc_task` → `TICKET.*`
- `change_request`, `change_task` → `TICKET.*` with `HIGH` risk
- `sys_user`, `sys_user_role` → `ACCESS.*`
- `cmdb_ci`, `cmdb_rel_ci` → `DATA.*` with `HIGH` risk
- `sys_audit`, `sys_log` → `DATA.READ` with `HIGH` risk (read-only enforcement)

**Implementation effort:** ~3 days. Table-name risk overrides are the core challenge — requires a lookup table of sensitive ServiceNow table names and their risk tier mappings.

---

### Integration 8 — Microsoft Teams (Rank: Medium-Hard)

**Why medium-hard:** Teams governance is done via the Microsoft Graph API (`graph.microsoft.com`). The Graph API is enormous — governance must be scoped to the Teams-relevant subset. OAuth 2.0 (client credentials or delegated) is more complex than API key auth and will require documenting the credential setup for operators.

**Microsoft Graph API — Teams subset (`graph.microsoft.com/v1.0`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/v1.0/teams/{teamId}/channels` | `DATA.QUERY` | `LOW` |
| `POST` | `/v1.0/teams/{teamId}/channels` | `ACCESS.GRANT` | `HIGH` |
| `DELETE` | `/v1.0/teams/{teamId}/channels/{channelId}` | `DATA.DELETE` | `CRITICAL` |
| `POST` | `/v1.0/teams/{teamId}/channels/{channelId}/messages` | `NOTIFICATION.SEND` | `MEDIUM` |
| `PATCH` | `/v1.0/teams/{teamId}/channels/{channelId}/messages/{messageId}` | `NOTIFICATION.UPDATE` | `LOW` |
| `POST` | `/v1.0/chats/{chatId}/messages` | `NOTIFICATION.SEND` | `MEDIUM` |
| `GET` | `/v1.0/teams/{teamId}/members` | `DATA.QUERY` | `LOW` |
| `POST` | `/v1.0/teams/{teamId}/members` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/v1.0/teams/{teamId}/members/{membershipId}` | `ACCESS.GRANT` | `CRITICAL` |
| `POST` | `/v1.0/teams` | `ACCESS.GRANT` | `CRITICAL` |
| `GET` | `/v1.0/groups/{groupId}/members` | `DATA.QUERY` | `LOW` |
| `POST` | `/v1.0/groups/{groupId}/members/$ref` | `ACCESS.GRANT` | `CRITICAL` |

**Auth type:** `bearer` (Azure AD access token). The auth flow (client credentials grant) must be handled externally and the resulting access token stored as the `auth_value`. The connector itself stays the same — rbitr stores the token, not the credential flow.

**Classifier guard:** `isMSGraphPath` checks `segments[0] == "v1.0"` or `segments[0] == "beta"`.

**Implementation effort:** ~2.5 days classifier + tests. Additional ~1 day operator documentation for Azure app registration and token provisioning.

---

### Integration 9 — Salesforce (Rank: Hard)

**Why hard:** Salesforce uses OAuth 2.0 with instance-specific base URLs (`{instance}.salesforce.com`). The REST API is clean but the Salesforce data model is complex (standard + custom objects). SOQL queries require query parameter classification in addition to path classification.

**Salesforce REST API (`{instance}.salesforce.com/services/data/v{version}`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/services/data/v{v}/sobjects/{sObject}` | `CRM.READ` | `LOW` |
| `POST` | `/services/data/v{v}/sobjects/{sObject}` | `DATA.UPDATE` | `MEDIUM` |
| `GET` | `/services/data/v{v}/sobjects/{sObject}/{id}` | `CRM.READ` | `LOW` |
| `PATCH` | `/services/data/v{v}/sobjects/{sObject}/{id}` | `DATA.UPDATE` | `MEDIUM` |
| `DELETE` | `/services/data/v{v}/sobjects/{sObject}/{id}` | `CRM.DELETE` | `CRITICAL` |
| `GET` | `/services/data/v{v}/query?q={SOQL}` | `DATA.QUERY` | `LOW` |
| `GET` | `/services/data/v{v}/queryAll?q={SOQL}` | `DATA.BULK_EXPORT` | `CRITICAL` |
| `POST` | `/services/data/v{v}/composite/batch` | `DATA.UPDATE` | `HIGH` |
| `POST` | `/services/data/v{v}/composite/sobjects` | `DATA.UPDATE` | `CRITICAL` |
| `GET` | `/services/data/v{v}/analytics/reports/{id}` | `DATA.EXPORT` | `CRITICAL` |

**Classifier guard:** `isSalesforcePath` checks `segments[0] == "services" && segments[1] == "data"`.

**Risk considerations:**
- `queryAll` retrieves soft-deleted records — maps to `DATA.BULK_EXPORT` (CRITICAL). This is important for data governance.
- `composite/sobjects` batch upsert must be `CRITICAL`.
- CRM object sensitivity overrides: `Lead`, `Contact`, `Opportunity` should be `MEDIUM`+ vs custom objects.

**Implementation effort:** ~3 days. The primary challenge is SOQL endpoint detection in query parameters and the distinction between read-only queries and destructive batch operations.

---

### Integration 10 — Okta (Rank: Hard)

**Why hard:** Okta is an identity provider — all operations are security-critical. The action taxonomy maps well (`ACCESS.*`), but getting the risk tiers right requires careful thought. The API is clean REST but the blast radius of mistakes is high, which demands thorough test coverage.

**Okta Management API (`{okta-domain}.okta.com/api/v1`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/api/v1/users` | `DATA.QUERY` | `MEDIUM` |
| `POST` | `/api/v1/users` | `ACCESS.GRANT` | `CRITICAL` |
| `GET` | `/api/v1/users/{userId}` | `DATA.READ` | `LOW` |
| `PUT` | `/api/v1/users/{userId}` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `POST` | `/api/v1/users/{userId}/lifecycle/activate` | `ACCESS.GRANT` | `CRITICAL` |
| `POST` | `/api/v1/users/{userId}/lifecycle/deactivate` | `ACCESS.GRANT` | `CRITICAL` |
| `DELETE` | `/api/v1/users/{userId}` | `DATA.DELETE` | `CRITICAL` |
| `GET` | `/api/v1/groups` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/groups` | `ACCESS.GRANT` | `CRITICAL` |
| `PUT` | `/api/v1/groups/{groupId}/users/{userId}` | `ACCESS.ROLE_CHANGE` | `CRITICAL` |
| `DELETE` | `/api/v1/groups/{groupId}/users/{userId}` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `GET` | `/api/v1/roles` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/users/{userId}/roles` | `ACCESS.ROLE_CHANGE` | `CRITICAL` |
| `DELETE` | `/api/v1/users/{userId}/roles/{roleId}` | `ACCESS.ROLE_CHANGE` | `HIGH` |
| `GET` | `/api/v1/apps` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/apps` | `ACCESS.GRANT` | `CRITICAL` |
| `POST` | `/api/v1/apps/{appId}/users` | `ACCESS.GRANT` | `CRITICAL` |

**Auth type:** `api_key` (`Authorization: SSWS {apiToken}`). No new connector logic.

**Classifier guard:** `isOktaPath` checks `segments[0] == "api" && segments[1] == "v1"`. A lifecycle path (`lifecycle/activate`, `lifecycle/deactivate`, `lifecycle/suspend`, `lifecycle/unsuspend`) requires explicit classification at `ACCESS.GRANT` / `CRITICAL`.

**Default policy override:** All `ACCESS.*` actions should default to `REQUIRE_APPROVAL` for Okta (not just `DENY`). An OPA policy template for Okta should be shipped as part of this integration, as the identity blast radius justifies human-in-the-loop by default.

**Implementation effort:** ~3 days classifier + tests + Okta-specific policy template.

---

### Integration 11 — Datadog (Rank: Hard)

**Why hard:** Datadog spans a wide surface: metrics, logs, traces, monitors, incidents, dashboards. The governance value is in the write paths (create monitor, silence alert, modify dashboard) and the export paths (bulk log/metric export). The API is clean REST but two API versions are in active use (v1 and v2 for different resource types).

**Datadog API (`api.datadoghq.com`):**

| HTTP Method | Path Pattern | Action Type | Risk |
|---|---|---|---|
| `GET` | `/api/v1/monitor` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/monitor` | `TICKET.CREATE` | `MEDIUM` |
| `PUT` | `/api/v1/monitor/{monitorId}` | `TICKET.UPDATE` | `MEDIUM` |
| `DELETE` | `/api/v1/monitor/{monitorId}` | `DATA.DELETE` | `HIGH` |
| `POST` | `/api/v1/downtime` | `TICKET.UPDATE` | `HIGH` |
| `GET` | `/api/v1/dashboard` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/dashboard` | `DATA.UPDATE` | `LOW` |
| `DELETE` | `/api/v1/dashboard/{dashboardId}` | `DATA.DELETE` | `HIGH` |
| `POST` | `/api/v2/incidents` | `TICKET.CREATE` | `HIGH` |
| `PATCH` | `/api/v2/incidents/{incidentId}` | `TICKET.UPDATE` | `MEDIUM` |
| `GET` | `/api/v2/logs/events` | `DATA.QUERY` | `MEDIUM` |
| `POST` | `/api/v2/logs/analytics/aggregate` | `DATA.EXPORT` | `CRITICAL` |
| `GET` | `/api/v2/metrics` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v2/metrics/{metricName}/volumes` | `DATA.EXPORT` | `CRITICAL` |
| `GET` | `/api/v1/users` | `DATA.QUERY` | `LOW` |
| `POST` | `/api/v1/user` | `ACCESS.GRANT` | `CRITICAL` |

**Auth type:** `api_key` (`DD-API-KEY` header + optional `DD-APPLICATION-KEY`). Requires dual-header auth support — the generic connector currently uses a single `auth_value`. This needs a connector enhancement or a convention for encoding both keys.

**New action types required:** None (reuses existing taxonomy).

**Implementation effort:** ~3.5 days. The dual-header auth requirement is a connector-level change that benefits other integrations too.

---

### Integration 12 — AWS (via Service Endpoints) (Rank: Hardest)

**Why hardest:** AWS uses AWS Signature Version 4 (SigV4) — a bespoke HMAC-based signing scheme that is fundamentally different from `bearer` or `api_key`. Implementing SigV4 requires computing a canonical request, credential scope, and signing key per request using the request timestamp, body hash, and IAM credentials. This is a connector-level change, not just a classifier change.

Additionally, AWS has service-specific base URLs (`ec2.amazonaws.com`, `iam.amazonaws.com`, `s3.amazonaws.com`), so tool configs need service-level granularity.

**Priority AWS Services for Governance:**

**IAM (`iam.amazonaws.com`):**

| Action | Governance | Action Type | Risk |
|---|---|---|---|
| `CreateUser` | `ACCESS.GRANT` | `CRITICAL` | |
| `DeleteUser` | `DATA.DELETE` | `CRITICAL` | |
| `AttachUserPolicy` | `ACCESS.ROLE_CHANGE` | `CRITICAL` | |
| `CreateRole` | `ACCESS.GRANT` | `CRITICAL` | |
| `AttachRolePolicy` | `ACCESS.ROLE_CHANGE` | `CRITICAL` | |
| `CreateAccessKey` | `ACCESS.GRANT` | `CRITICAL` | |
| `DeleteAccessKey` | `DATA.DELETE` | `HIGH` | |

**S3 (`s3.amazonaws.com` or path-style):**

| Action | Governance | Action Type | Risk |
|---|---|---|---|
| `GetObject` | `DATA.READ` | `LOW` | |
| `PutObject` | `DATA.UPDATE` | `MEDIUM` | |
| `DeleteObject` | `DATA.DELETE` | `HIGH` | |
| `GetObject` (with `--recursive`) | `DATA.BULK_EXPORT` | `CRITICAL` | |
| `PutBucketPolicy` | `ACCESS.GRANT` | `CRITICAL` | |
| `DeleteBucket` | `DATA.DELETE` | `CRITICAL` | |

**EC2 / Systems Manager (AWS Query API format):**

AWS uses a query-string action format (`Action=DescribeInstances`) or JSON body actions (`ssm:SendCommand`) — the classifier needs to inspect the `Action` query parameter or body key, not just the path.

**Auth type:** SigV4 — requires new connector type `aws_sigv4` with fields `aws_access_key_id`, `aws_secret_access_key`, `aws_region`, `aws_service`. This is a substantial connector-level addition.

**Implementation effort:** ~6-8 days total:
- ~2 days: SigV4 connector implementation + tests
- ~2 days: IAM + S3 classifier with query/body action routing
- ~2 days: EC2/SSM classifier
- ~1 day: Seed migrations + integration tests
- ~1 day: Operator documentation (IAM least-privilege setup for rbitr connector credentials)

---

## Part 3 — New Action Types Required

The following new action types should be added to the taxonomy in this epic:

| Action Type | Domain | Default Risk | Description |
|---|---|---|---|
| `NOTIFICATION.SEND` | NOTIFICATION | `MEDIUM` | Send a message to a channel, user, or group |
| `NOTIFICATION.UPDATE` | NOTIFICATION | `LOW` | Edit or update a previously sent message |

These are needed for Slack, HubSpot marketing emails, and Microsoft Teams. The OPA default policy should map `NOTIFICATION.SEND` to `ALLOW` for standard channels but `REQUIRE_APPROVAL` for broadcast/all-users sends (configurable via argument constraints on channel scope).

---

## Part 4 — Implementation Architecture

Every integration follows the same three-layer pattern:

### Layer 1 — Classifier

Add a new `classify{Integration}` function in `internal/classification/classifier.go` and register it in `Classify()` by `toolID`:

```go
if toolID == "github" {
    return classifyGitHub(&input)
}
```

Each integration classifier:
1. Has a path guard (`is{Integration}Path`) that validates the base path structure.
2. Checks most-specific operations first (create, comment, delete), falling through to `classifyGeneric`.
3. Has table-driven unit tests covering all key endpoint patterns.

### Layer 2 — Seed Migration

Add a numbered migration (`000XX_seed_{integration}_tool_metadata.sql`) that:
1. Inserts the tool record for the `t_demo` tenant.
2. Sets `description` and `input_schema_json` for MCP `tools/list` compatibility.
3. Uses `ON CONFLICT DO UPDATE` for idempotency.

### Layer 3 — Dev Auto-Tool Wiring

Add the new tool to `insertDevTools` in `internal/api/setup/service.go` under `RBTR_DEV_AUTO_TOOLS=true`, with a corresponding `RBTR_DEV_{INTEGRATION}_URL` env var for local stub server pointing.

---

## Part 5 — Stories

### Story 1 — Jira Classifier Fixes (P0)

Fix the gaps identified in Part 1 of this epic:

- Add `isJiraAgile` path guard for `/rest/agile/1.0/...` endpoints.
- Fix issue link classification (`issuelink` token → `TICKET.UPDATE`).
- Fix watcher removal from `DATA.DELETE` to `TICKET.UPDATE`.
- Fix archive classification from `DATA.EXPORT` to `TICKET.UPDATE`.
- Add explicit attachment handling.
- Add classifier tests for all fixed cases.

Acceptance criteria:
- All fixed endpoints produce the correct `action_type`, `action_risk`, and `action_summary`.
- No regressions on existing classifier test cases.

---

### Story 2 — Add `NOTIFICATION.*` Action Types (P0)

Add two new action type constants to the taxonomy and wire them into:
- `defaultRisk` mapping
- OPA default policy module in `internal/api/setup/service.go`
- Any existing evidence/audit export whitelists

Acceptance criteria:
- `NOTIFICATION.SEND` defaults to `ALLOW` in the default policy.
- `NOTIFICATION.SEND` can be overridden to `REQUIRE_APPROVAL` via argument constraints on channel/recipient fields.

---

### Story 3 — Confluence Integration (P1)

Deliverables:
- `classifyConfluence` classifier function + `isConfluencePath` guard.
- Seed migration for `confluence` tool with description and input schema.
- Dev auto-tool wiring via `RBTR_DEV_CONFLUENCE_URL`.
- Table-driven classifier tests.

---

### Story 4 — GitHub Integration (P1)

Deliverables:
- `classifyGitHub` classifier + `isGitHubPath` guard.
- Seed migration for `github` tool.
- Dev auto-tool wiring via `RBTR_DEV_GITHUB_URL`.
- Classifier tests covering issue create, PR merge (HIGH risk), collaborator add (CRITICAL).

---

### Story 5 — PagerDuty + Opsgenie Integration (P1)

Deliverables:
- `classifyPagerDuty` + `classifyOpsgenie` classifiers.
- Seed migrations for both tools.
- Dev auto-tool wiring.
- Classifier tests.

These two are bundled as one story given their structural similarity.

---

### Story 6 — Slack Integration (P2)

Deliverables:
- `classifySlack` classifier using method-name lookup table strategy.
- Seed migration with input schema enumerating supported Slack methods.
- Classifier tests covering `chat.postMessage`, `conversations.create`, `users.admin.setOwner`.

---

### Story 7 — HubSpot Integration (P2)

Deliverables:
- `classifyHubSpot` classifier + `isHubSpotPath` guard.
- Seed migration.
- Classifier tests covering CRM CRUD, batch archive, and marketing email send.

---

### Story 8 — ServiceNow Integration (P2)

Deliverables:
- `classifyServiceNow` classifier with table-name risk override lookup.
- `servicenowSensitiveTables` map: `incident`, `change_request`, `sys_user`, `cmdb_ci`, etc.
- Seed migration.
- Classifier tests covering incident create, user create (CRITICAL), CMDB update (HIGH).

---

### Story 9 — Microsoft Teams Integration (P3)

Deliverables:
- `classifyMSGraph` classifier + `isMSGraphPath` guard scoped to Teams resources.
- Seed migration.
- Operator documentation: Azure app registration, client credentials grant, token provisioning into rbitr tool config.
- Classifier tests.

---

### Story 10 — Salesforce Integration (P3)

Deliverables:
- `classifySalesforce` classifier with `queryAll` bulk export detection.
- Seed migration.
- Classifier tests covering SOQL query, bulk upsert, analytics report export.

---

### Story 11 — Okta Integration (P3)

Deliverables:
- `classifyOkta` classifier with lifecycle endpoint detection.
- Okta-specific OPA policy template (all `ACCESS.*` default to `REQUIRE_APPROVAL` for Okta toolID).
- Seed migration.
- Classifier tests covering user provisioning, group membership, role assignment.
- Security review gate before shipping (given identity blast radius).

---

### Story 12 — Datadog Integration (P3)

Deliverables:
- `classifyDatadog` classifier (v1 + v2 paths).
- Dual-header auth support in generic connector (`DD-API-KEY` + `DD-APPLICATION-KEY`).
- Seed migration.
- Classifier tests covering monitor create, log aggregate export (CRITICAL).

---

### Story 13 — AWS Integration (P4, Stretch)

Deliverables:
- `aws_sigv4` connector type with SigV4 request signing.
- `classifyAWS` classifier routing on Action query parameter (IAM, S3) or body action (SSM).
- Seed migrations for `aws_iam`, `aws_s3`, `aws_ec2` tools.
- Operator documentation: IAM policy for rbitr connector credentials (least privilege).
- Classifier tests covering IAM `AttachRolePolicy` (CRITICAL), S3 `DeleteBucket` (CRITICAL), SSM `SendCommand` (HIGH).

This is a stretch story and may span multiple sub-epics given the SigV4 connector work.

---

## Part 6 — Ranked Summary

| Rank | Integration | Difficulty | Auth | New Action Types | Connector Changes |
|------|-------------|------------|------|-----------------|-------------------|
| 1 | Confluence | Easiest | `bearer` (same as Jira) | None | None |
| 2 | GitHub | Easy | `bearer` | None | None |
| 3 | PagerDuty | Easy-Medium | `api_key` | None | None |
| 4 | Opsgenie | Easy-Medium | `api_key` | None | None |
| 5 | Slack | Medium | `bearer` | `NOTIFICATION.*` | None |
| 6 | HubSpot | Medium | `bearer` | `NOTIFICATION.SEND` (reuse) | None |
| 7 | ServiceNow | Medium-Hard | `bearer` / basic | None | None |
| 8 | Microsoft Teams | Medium-Hard | `bearer` (Azure AD) | `NOTIFICATION.*` (reuse) | None |
| 9 | Salesforce | Hard | `bearer` (OAuth 2.0) | None | None |
| 10 | Okta | Hard | `api_key` | None | None + policy template |
| 11 | Datadog | Hard | `api_key` (dual-header) | None | Dual-header auth support |
| 12 | AWS | Hardest | SigV4 | None | New `aws_sigv4` connector |

---

## Definition of Done

- Story 1 (Jira fixes) complete with tests and no regressions.
- All P1 integrations (Confluence, GitHub, PagerDuty, Opsgenie) classified and seeded.
- `NOTIFICATION.*` action types in taxonomy, default policy, and OPA module.
- Each integration has a seed migration usable with `RBTR_DEV_AUTO_TOOLS=true`.
- Each integration classifier has minimum 80% coverage on meaningful endpoint patterns.
- All integrations work end-to-end through MCP `tools/call` governed execution path.
- Operator documentation covers auth provisioning for each integration.

## Non-Goals for Epic 10

- Full OAuth 2.0 authorization code flow in rbitr (operators manage token provisioning externally; rbitr stores the resulting bearer token).
- Integration-specific webhook receivers (inbound notifications from integrations).
- Native integration UIs beyond tool config management already in the admin console.
- SaaS marketplace listings for individual integrations (separate GTM epic).
