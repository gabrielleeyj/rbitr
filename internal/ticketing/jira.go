package ticketing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// JiraProvider implements TicketProvider using Jira REST API v3.
type JiraProvider struct {
	baseURL  string
	apiToken string
	email    string
	client   *http.Client
}

func NewJiraProvider(baseURL, apiToken, email string) *JiraProvider {
	return &JiraProvider{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiToken: apiToken,
		email:    email,
		client:   http.DefaultClient,
	}
}

func (p *JiraProvider) Name() string { return ProviderJira }

func (p *JiraProvider) CreateTicket(ctx context.Context, req *CreateTicketRequest) (CreateTicketResult, error) {
	payload := map[string]any{
		"fields": map[string]any{
			"project":     map[string]string{"key": req.ProjectKey},
			"summary":     req.Summary,
			"description": jiraDescription(req.Description),
			"issuetype":   map[string]string{"name": req.IssueType},
			"priority":    map[string]string{"name": req.Priority},
		},
	}
	if len(req.Labels) > 0 {
		if fields, ok := payload["fields"].(map[string]any); ok {
			fields["labels"] = req.Labels
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("jira: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/rest/api/3/issue", bytes.NewReader(body))
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("jira: create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("jira: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, readLimitBytes))
		return CreateTicketResult{}, fmt.Errorf("jira: create issue failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CreateTicketResult{}, fmt.Errorf("jira: decode response: %w", err)
	}

	return CreateTicketResult{
		ExternalKey: result.Key,
		ExternalURL: p.baseURL + "/browse/" + result.Key,
	}, nil
}

func (p *JiraProvider) UpdateTicket(ctx context.Context, req *UpdateTicketRequest) error {
	if req.Comment != "" {
		if err := p.addComment(ctx, req.ExternalKey, req.Comment); err != nil {
			return err
		}
	}
	if req.Status != "" {
		return p.transitionIssue(ctx, req.ExternalKey, req.Status)
	}
	return nil
}

func (p *JiraProvider) addComment(ctx context.Context, issueKey, comment string) error {
	payload := map[string]any{
		"body": jiraDescription(comment),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("jira: marshal comment: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", p.baseURL, issueKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jira: create comment request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("jira: send comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("jira: add comment failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (p *JiraProvider) transitionIssue(ctx context.Context, issueKey, targetStatus string) error {
	transitionID, err := p.findTransition(ctx, issueKey, targetStatus)
	if err != nil {
		return err
	}
	if transitionID == "" {
		return nil
	}

	payload := map[string]any{
		"transition": map[string]string{"id": transitionID},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("jira: marshal transition: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", p.baseURL, issueKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jira: create transition request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("jira: send transition: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira: transition failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (p *JiraProvider) findTransition(ctx context.Context, issueKey, targetStatus string) (string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", p.baseURL, issueKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("jira: create transitions request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("jira: get transitions: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Transitions []struct {
			ID string `json:"id"`
			To struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("jira: decode transitions: %w", err)
	}

	for _, t := range result.Transitions {
		if strings.EqualFold(t.To.Name, targetStatus) {
			return t.ID, nil
		}
	}
	return "", nil
}

func (p *JiraProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if p.email != "" {
		req.SetBasicAuth(p.email, p.apiToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.apiToken)
	}
}

func jiraDescription(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	}
}
