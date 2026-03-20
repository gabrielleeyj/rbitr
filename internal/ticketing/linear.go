package ticketing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	linearAPIURL    = "https://api.linear.app/graphql"
	readLimitBytes  = 1024
	identifierParts = 2
	linearPriUrgent = 1
	linearPriHigh   = 2
	linearPriMedium = 3
	linearPriLow    = 4
)

var (
	errLinearUnexpectedResponse = errors.New("linear: unexpected response structure")
	errLinearMissingIssue       = errors.New("linear: missing issue in response")
	errLinearUnexpected         = errors.New("linear: unexpected response")
)

// LinearProvider implements TicketProvider using Linear's GraphQL API.
type LinearProvider struct {
	apiToken string
	apiURL   string
	client   *http.Client
}

func NewLinearProvider(apiToken string) *LinearProvider {
	return &LinearProvider{
		apiToken: apiToken,
		apiURL:   linearAPIURL,
		client:   http.DefaultClient,
	}
}

func (p *LinearProvider) Name() string { return ProviderLinear }

func (p *LinearProvider) CreateTicket(ctx context.Context, req *CreateTicketRequest) (CreateTicketResult, error) {
	priorityNum := linearPriorityNumber(req.Priority)

	query := `mutation IssueCreate($input: IssueCreateInput!) {
		issueCreate(input: $input) {
			success
			issue {
				identifier
				url
			}
		}
	}`

	variables := map[string]any{
		"input": map[string]any{
			"teamId":      req.ProjectKey,
			"title":       req.Summary,
			"description": req.Description,
			"priority":    priorityNum,
		},
	}
	if len(req.Labels) > 0 {
		if input, ok := variables["input"].(map[string]any); ok {
			input["labelIds"] = req.Labels
		}
	}

	result, err := p.graphql(ctx, query, variables)
	if err != nil {
		return CreateTicketResult{}, err
	}

	issueCreate, ok := result["issueCreate"].(map[string]any)
	if !ok {
		return CreateTicketResult{}, errLinearUnexpectedResponse
	}
	issue, ok := issueCreate["issue"].(map[string]any)
	if !ok {
		return CreateTicketResult{}, errLinearMissingIssue
	}

	identifier, _ := issue["identifier"].(string)
	issueURL, _ := issue["url"].(string)

	return CreateTicketResult{
		ExternalKey: identifier,
		ExternalURL: issueURL,
	}, nil
}

func (p *LinearProvider) UpdateTicket(ctx context.Context, req *UpdateTicketRequest) error {
	issueID, err := p.lookupIssueID(ctx, req.ExternalKey)
	if err != nil {
		return err
	}

	if req.Comment != "" {
		if commentErr := p.addComment(ctx, issueID, req.Comment); commentErr != nil {
			return commentErr
		}
	}

	if req.Status != "" {
		return p.updateState(ctx, issueID, req.Status)
	}
	return nil
}

func (p *LinearProvider) lookupIssueID(ctx context.Context, identifier string) (string, error) {
	parts := strings.SplitN(identifier, "-", identifierParts)
	if len(parts) != identifierParts {
		return "", fmt.Errorf("linear: invalid identifier format: %s", identifier)
	}

	query := `query Issue($filter: IssueFilter) {
		issues(filter: $filter, first: 1) {
			nodes { id }
		}
	}`
	variables := map[string]any{
		"filter": map[string]any{
			"number": map[string]any{"eq": parts[1]},
			"team":   map[string]any{"key": map[string]any{"eq": parts[0]}},
		},
	}

	result, err := p.graphql(ctx, query, variables)
	if err != nil {
		return "", err
	}

	issues, ok := result["issues"].(map[string]any)
	if !ok {
		return "", errLinearUnexpected
	}
	nodes, ok := issues["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		return "", fmt.Errorf("linear: issue %s not found", identifier)
	}
	node, _ := nodes[0].(map[string]any)
	id, _ := node["id"].(string)
	return id, nil
}

func (p *LinearProvider) addComment(ctx context.Context, issueID, body string) error {
	query := `mutation CommentCreate($input: CommentCreateInput!) {
		commentCreate(input: $input) { success }
	}`
	variables := map[string]any{
		"input": map[string]any{
			"issueId": issueID,
			"body":    body,
		},
	}
	_, err := p.graphql(ctx, query, variables)
	return err
}

func (p *LinearProvider) updateState(ctx context.Context, issueID, status string) error {
	stateID, err := p.findStateID(ctx, issueID, status)
	if err != nil || stateID == "" {
		return err
	}

	query := `mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) {
		issueUpdate(id: $id, input: $input) { success }
	}`
	variables := map[string]any{
		"id":    issueID,
		"input": map[string]any{"stateId": stateID},
	}
	_, err = p.graphql(ctx, query, variables)
	return err
}

func (p *LinearProvider) findStateID(ctx context.Context, issueID, targetStatus string) (string, error) {
	query := `query Issue($id: String!) {
		issue(id: $id) {
			team {
				states { nodes { id name } }
			}
		}
	}`
	variables := map[string]any{"id": issueID}

	result, err := p.graphql(ctx, query, variables)
	if err != nil {
		return "", err
	}

	issue, _ := result["issue"].(map[string]any)
	team, _ := issue["team"].(map[string]any)
	states, _ := team["states"].(map[string]any)
	nodes, _ := states["nodes"].([]any)

	for _, n := range nodes {
		state, _ := n.(map[string]any)
		name, _ := state["name"].(string)
		if strings.EqualFold(name, targetStatus) {
			id, _ := state["id"].(string)
			return id, nil
		}
	}
	return "", nil
}

func (p *LinearProvider) graphql(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("linear: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linear: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", p.apiToken)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("linear: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, readLimitBytes))
		return nil, fmt.Errorf("linear: request failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gqlResp struct {
		Data   map[string]any   `json:"data"`
		Errors []map[string]any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("linear: decode response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		msg, _ := gqlResp.Errors[0]["message"].(string)
		return nil, fmt.Errorf("linear: graphql error: %s", msg)
	}
	return gqlResp.Data, nil
}

func linearPriorityNumber(priority string) int {
	switch priority {
	case "Urgent":
		return linearPriUrgent
	case priorityHigh:
		return linearPriHigh
	case priorityMedium:
		return linearPriMedium
	case priorityLow:
		return linearPriLow
	default:
		return 0
	}
}
