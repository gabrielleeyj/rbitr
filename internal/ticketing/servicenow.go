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

// ServiceNowProvider implements TicketProvider using ServiceNow REST Table API.
type ServiceNowProvider struct {
	baseURL  string
	apiToken string
	client   *http.Client
}

func NewServiceNowProvider(baseURL, apiToken string) *ServiceNowProvider {
	return &ServiceNowProvider{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiToken: apiToken,
		client:   http.DefaultClient,
	}
}

func (p *ServiceNowProvider) Name() string { return ProviderServiceNow }

func (p *ServiceNowProvider) CreateTicket(ctx context.Context, req *CreateTicketRequest) (CreateTicketResult, error) {
	tableName := req.IssueType
	if tableName == "" {
		tableName = "incident"
	}

	payload := map[string]string{
		"short_description": req.Summary,
		"description":       req.Description,
		fieldPriority:       req.Priority,
		"assignment_group":  req.ProjectKey,
		"category":          "rbitr-approval",
	}
	if val, ok := req.Metadata["approval_request_id"]; ok {
		payload["correlation_id"] = val
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("servicenow: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/now/table/%s", p.baseURL, tableName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("servicenow: create request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return CreateTicketResult{}, fmt.Errorf("servicenow: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, readLimitBytes))
		return CreateTicketResult{}, fmt.Errorf("servicenow: create failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result struct {
			SysID  string `json:"sys_id"`
			Number string `json:"number"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CreateTicketResult{}, fmt.Errorf("servicenow: decode response: %w", err)
	}

	return CreateTicketResult{
		ExternalKey: result.Result.Number,
		ExternalURL: fmt.Sprintf("%s/nav_to.do?uri=%s.do?sys_id=%s", p.baseURL, tableName, result.Result.SysID),
	}, nil
}

func (p *ServiceNowProvider) UpdateTicket(ctx context.Context, req *UpdateTicketRequest) error {
	sysID, err := p.lookupSysID(ctx, req.ExternalKey)
	if err != nil {
		return err
	}

	payload := map[string]string{}
	if req.Status != "" {
		payload["state"] = mapServiceNowState(req.Status)
	}
	if req.Comment != "" {
		payload["work_notes"] = req.Comment
	}
	if len(payload) == 0 {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("servicenow: marshal update: %w", err)
	}

	url := fmt.Sprintf("%s/api/now/table/incident/%s", p.baseURL, sysID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("servicenow: create update request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("servicenow: send update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servicenow: update failed (status %d)", resp.StatusCode)
	}
	return nil
}

func (p *ServiceNowProvider) lookupSysID(ctx context.Context, number string) (string, error) {
	url := fmt.Sprintf("%s/api/now/table/incident?sysparm_query=number=%s&sysparm_fields=sys_id&sysparm_limit=1", p.baseURL, number)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("servicenow: create lookup request: %w", err)
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("servicenow: send lookup: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			SysID string `json:"sys_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("servicenow: decode lookup: %w", err)
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("servicenow: incident %s not found", number)
	}
	return result.Result[0].SysID, nil
}

func (p *ServiceNowProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
}

func mapServiceNowState(status string) string {
	switch status {
	case TicketStatusResolved:
		return "6"
	case TicketStatusClosed:
		return "7"
	case TicketStatusInProgress:
		return "2"
	default:
		return "1"
	}
}
