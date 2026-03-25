package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

// ImportMode controls how OpenAPI operations map to tools.
type ImportMode string

const (
	ModeSingle ImportMode = "single"
	ModeMulti  ImportMode = "multi"
)

// ImportRequest contains the parameters for an OpenAPI import.
type ImportRequest struct {
	SpecURL         string     `json:"spec_url"`
	SpecBody        []byte     `json:"-"` // raw JSON/YAML if uploaded directly
	Mode            ImportMode `json:"mode"`
	BaseURLOverride string     `json:"base_url_override"`
	AuthType        string     `json:"auth_type"`
	AuthValue       string     `json:"auth_value"`
	Prefix          string     `json:"prefix"`
}

// GeneratedTool represents a tool definition produced from an OpenAPI spec.
type GeneratedTool struct {
	ToolID             string          `json:"tool_id"`
	Description        string          `json:"description"`
	BaseURL            string          `json:"base_url"`
	Transport          string          `json:"transport"`
	AuthType           string          `json:"auth_type"`
	InputSchemaJSON    json.RawMessage `json:"input_schema_json"`
	OpenAPISpecURL     string          `json:"openapi_spec_url"`
	OpenAPIOperationID string          `json:"openapi_operation_id,omitempty"`
}

// ParseAndGenerate loads an OpenAPI spec and generates tool definitions.
func ParseAndGenerate(ctx context.Context, req ImportRequest) ([]GeneratedTool, error) {
	doc, err := loadSpec(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	baseURL := resolveBaseURL(doc, req.BaseURLOverride)
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL: set base_url_override or add servers to the spec")
	}

	authType := req.AuthType
	if authType == "" {
		authType = "none"
	}

	switch req.Mode {
	case ModeSingle:
		return generateSingleTool(doc, req, baseURL, authType)
	case ModeMulti:
		return generateMultiTools(doc, req, baseURL, authType)
	default:
		return nil, fmt.Errorf("invalid mode: %s (must be 'single' or 'multi')", req.Mode)
	}
}

// ToModels converts generated tools into models.Tool structs ready for insertion.
func ToModels(tools []GeneratedTool, tenantID, authValue string) []models.Tool {
	result := make([]models.Tool, 0, len(tools))
	for _, gt := range tools {
		result = append(result, models.Tool{
			ToolID:             gt.ToolID,
			TenantID:           tenantID,
			BaseURL:            gt.BaseURL,
			AuthType:           gt.AuthType,
			AuthValue:          authValue,
			Transport:          gt.Transport,
			Description:        gt.Description,
			InputSchemaJSON:    gt.InputSchemaJSON,
			Source:             "openapi_import",
			OpenAPISpecURL:     gt.OpenAPISpecURL,
			OpenAPIOperationID: gt.OpenAPIOperationID,
		})
	}
	return result
}

func loadSpec(ctx context.Context, req ImportRequest) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	if len(req.SpecBody) > 0 {
		return loader.LoadFromData(req.SpecBody)
	}

	if req.SpecURL == "" {
		return nil, fmt.Errorf("spec_url or spec body required")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodGet, req.SpecURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid spec_url: %w", err)
	}
	resp, err := httpClient.Do(reqHTTP)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spec endpoint returned %d", resp.StatusCode)
	}

	return loader.LoadFromIoReader(resp.Body)
}

func resolveBaseURL(doc *openapi3.T, override string) string {
	if override != "" {
		return strings.TrimRight(override, "/")
	}
	if len(doc.Servers) > 0 && doc.Servers[0].URL != "" {
		return strings.TrimRight(doc.Servers[0].URL, "/")
	}
	return ""
}

// generateSingleTool creates one tool wrapping the entire API.
// The agent chooses path and method via input arguments.
func generateSingleTool(doc *openapi3.T, req ImportRequest, baseURL, authType string) ([]GeneratedTool, error) {
	paths, methods := collectPathsAndMethods(doc)
	if len(paths) == 0 {
		return nil, fmt.Errorf("spec contains no operations")
	}

	toolID := req.Prefix
	if toolID == "" {
		toolID = sanitizeToolID(doc.Info.Title)
	}

	desc := doc.Info.Description
	if desc == "" {
		desc = doc.Info.Title
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"enum":        paths,
				"description": "API endpoint path",
			},
			"method": map[string]any{
				"type":        "string",
				"enum":        methods,
				"description": "HTTP method",
			},
			"body": map[string]any{
				"type":        "object",
				"description": "Request body (JSON)",
			},
			"query": map[string]any{
				"type":        "object",
				"description": "Query parameters",
			},
		},
		"required": []string{"path"},
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	specURL := req.SpecURL

	return []GeneratedTool{{
		ToolID:          toolID,
		Description:     desc,
		BaseURL:         baseURL,
		Transport:       "http",
		AuthType:        authType,
		InputSchemaJSON: schemaJSON,
		OpenAPISpecURL:  specURL,
	}}, nil
}

// generateMultiTools creates one tool per OpenAPI operation.
func generateMultiTools(doc *openapi3.T, req ImportRequest, baseURL, authType string) ([]GeneratedTool, error) {
	var tools []GeneratedTool

	sortedPaths := sortedPathKeys(doc.Paths)

	for _, path := range sortedPaths {
		pathItem := doc.Paths.Value(path)
		if pathItem == nil {
			continue
		}

		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			op := pathItem.GetOperation(method)
			if op == nil {
				continue
			}

			operationID := op.OperationID
			if operationID == "" {
				operationID = strings.ToLower(method) + "_" + sanitizeToolID(path)
			}

			toolID := operationID
			if req.Prefix != "" {
				toolID = req.Prefix + "_" + operationID
			}

			desc := op.Summary
			if desc == "" {
				desc = op.Description
			}
			if desc == "" {
				desc = method + " " + path
			}

			schema := buildOperationSchema(pathItem, op, method, path)
			schemaJSON, err := json.Marshal(schema)
			if err != nil {
				return nil, err
			}

			tools = append(tools, GeneratedTool{
				ToolID:             toolID,
				Description:        desc,
				BaseURL:            baseURL,
				Transport:          "http",
				AuthType:           authType,
				InputSchemaJSON:    schemaJSON,
				OpenAPISpecURL:     req.SpecURL,
				OpenAPIOperationID: op.OperationID,
			})
		}
	}

	if len(tools) == 0 {
		return nil, fmt.Errorf("spec contains no operations")
	}

	return tools, nil
}

func buildOperationSchema(pathItem *openapi3.PathItem, op *openapi3.Operation, method, path string) map[string]any {
	properties := map[string]any{}
	var required []string

	// Merge path-level and operation-level parameters.
	allParams := append(pathItem.Parameters, op.Parameters...)

	for _, paramRef := range allParams {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		p := paramRef.Value
		prop := map[string]any{
			"type":        schemaType(p.Schema),
			"description": p.Description,
		}
		if p.Schema != nil && p.Schema.Value != nil {
			if len(p.Schema.Value.Enum) > 0 {
				prop["enum"] = p.Schema.Value.Enum
			}
		}

		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}

	// Request body → nested "body" property.
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		bodyProp := map[string]any{
			"type":        "object",
			"description": "Request body",
		}
		// Try to extract properties from JSON content schema.
		if ct := op.RequestBody.Value.Content.Get("application/json"); ct != nil && ct.Schema != nil && ct.Schema.Value != nil {
			bodySchema := schemaToMap(ct.Schema.Value)
			bodyProp = bodySchema
		}
		properties["body"] = bodyProp
		if op.RequestBody.Value.Required {
			required = append(required, "body")
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func schemaToMap(s *openapi3.Schema) map[string]any {
	m := map[string]any{
		"type": s.Type.Slice()[0],
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Enum) > 0 {
		m["enum"] = s.Enum
	}
	if s.Properties != nil {
		props := map[string]any{}
		for name, propRef := range s.Properties {
			if propRef.Value != nil {
				props[name] = schemaToMap(propRef.Value)
			}
		}
		m["properties"] = props
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	return m
}

func schemaType(ref *openapi3.SchemaRef) string {
	if ref == nil || ref.Value == nil {
		return "string"
	}
	types := ref.Value.Type.Slice()
	if len(types) > 0 {
		return types[0]
	}
	return "string"
}

func collectPathsAndMethods(doc *openapi3.T) ([]string, []string) {
	pathSet := map[string]bool{}
	methodSet := map[string]bool{}

	if doc.Paths == nil {
		return nil, nil
	}

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			if pathItem.GetOperation(method) != nil {
				pathSet[path] = true
				methodSet[method] = true
			}
		}
	}

	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	methods := make([]string, 0, len(methodSet))
	for m := range methodSet {
		methods = append(methods, m)
	}
	sort.Strings(methods)

	return paths, methods
}

func sortedPathKeys(paths *openapi3.Paths) []string {
	if paths == nil {
		return nil
	}
	m := paths.Map()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeToolID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if c == ' ' || c == '-' || c == '/' || c == '{' || c == '}' {
			b.WriteRune('_')
		}
	}
	result := b.String()
	// Collapse consecutive underscores.
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}
	result = strings.Trim(result, "_")
	if result == "" {
		result = "api"
	}
	return result
}
