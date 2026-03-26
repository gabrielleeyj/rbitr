package openapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func petStoreSpecBytes() []byte {
	return []byte(`{
  "openapi": "3.0.0",
  "info": {
    "title": "Pet Store",
    "description": "A sample pet store API",
    "version": "1.0.0"
  },
  "servers": [{"url": "https://petstore.example.com/v1"}],
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "parameters": [
          {
            "name": "limit",
            "in": "query",
            "required": false,
            "schema": {"type": "integer"}
          }
        ]
      },
      "post": {
        "operationId": "createPet",
        "summary": "Create a pet",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": {"type": "string"},
                  "tag": {"type": "string"}
                },
                "required": ["name"]
              }
            }
          }
        }
      }
    },
    "/pets/{petId}": {
      "get": {
        "operationId": "getPet",
        "summary": "Get a pet by ID",
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "schema": {"type": "string"}
          }
        ]
      }
    }
  }
}`)
}

func TestParseAndGenerate_SingleMode(t *testing.T) {
	req := ImportRequest{
		SpecBody: petStoreSpecBytes(),
		Mode:     ModeSingle,
		Prefix:   "petstore",
	}

	tools, err := ParseAndGenerate(context.Background(), &req)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	tool := tools[0]
	require.Equal(t, "petstore", tool.ToolID)
	require.Equal(t, "https://petstore.example.com/v1", tool.BaseURL)
	require.Equal(t, "http", tool.Transport)
	require.Equal(t, "none", tool.AuthType)
	require.Equal(t, "A sample pet store API", tool.Description)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.InputSchemaJSON, &schema))
	require.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	pathProp, ok := props["path"].(map[string]any)
	require.True(t, ok, "path property must be map[string]any")
	pathEnum, ok := pathProp["enum"].([]any)
	require.True(t, ok, "path enum must be []any")
	require.Contains(t, pathEnum, "/pets")
	require.Contains(t, pathEnum, "/pets/{petId}")

	methodProp, ok := props["method"].(map[string]any)
	require.True(t, ok, "method property must be map[string]any")
	methodEnum, ok := methodProp["enum"].([]any)
	require.True(t, ok, "method enum must be []any")
	require.Contains(t, methodEnum, "GET")
	require.Contains(t, methodEnum, "POST")
}

func TestParseAndGenerate_MultiMode(t *testing.T) {
	req := ImportRequest{
		SpecBody: petStoreSpecBytes(),
		Mode:     ModeMulti,
		Prefix:   "ps",
	}

	tools, err := ParseAndGenerate(context.Background(), &req)
	require.NoError(t, err)
	require.Len(t, tools, 3)

	// Tools should be sorted by path, then method.
	require.Equal(t, "ps_listPets", tools[0].ToolID)
	require.Equal(t, "ps_createPet", tools[1].ToolID)
	require.Equal(t, "ps_getPet", tools[2].ToolID)

	// Check listPets has query parameter.
	var listSchema map[string]any
	require.NoError(t, json.Unmarshal(tools[0].InputSchemaJSON, &listSchema))
	props, ok := listSchema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	require.Contains(t, props, "limit")

	// Check createPet has required body.
	var createSchema map[string]any
	require.NoError(t, json.Unmarshal(tools[1].InputSchemaJSON, &createSchema))
	createProps, ok := createSchema["properties"].(map[string]any)
	require.True(t, ok, "createPet properties must be map[string]any")
	require.Contains(t, createProps, "body")
	required, ok := createSchema["required"].([]any)
	require.True(t, ok, "required must be []any")
	require.Contains(t, required, "body")

	// Check getPet has required path param.
	var getSchema map[string]any
	require.NoError(t, json.Unmarshal(tools[2].InputSchemaJSON, &getSchema))
	getProps, ok := getSchema["properties"].(map[string]any)
	require.True(t, ok, "getPet properties must be map[string]any")
	require.Contains(t, getProps, "petId")
	getRequired, ok := getSchema["required"].([]any)
	require.True(t, ok, "getPet required must be []any")
	require.Contains(t, getRequired, "petId")
}

func TestParseAndGenerate_BaseURLOverride(t *testing.T) {
	req := ImportRequest{
		SpecBody:        petStoreSpecBytes(),
		Mode:            ModeSingle,
		BaseURLOverride: "https://custom.example.com/api",
	}

	tools, err := ParseAndGenerate(context.Background(), &req)
	require.NoError(t, err)
	require.Equal(t, "https://custom.example.com/api", tools[0].BaseURL)
}

func TestParseAndGenerate_AuthType(t *testing.T) {
	req := ImportRequest{
		SpecBody: petStoreSpecBytes(),
		Mode:     ModeSingle,
		AuthType: "bearer",
	}

	tools, err := ParseAndGenerate(context.Background(), &req)
	require.NoError(t, err)
	require.Equal(t, "bearer", tools[0].AuthType)
}

func TestParseAndGenerate_InvalidMode(t *testing.T) {
	req := ImportRequest{
		SpecBody: petStoreSpecBytes(),
		Mode:     "invalid",
	}

	_, err := ParseAndGenerate(context.Background(), &req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid mode")
}

func TestParseAndGenerate_NoSpec(t *testing.T) {
	req := ImportRequest{
		Mode: ModeSingle,
	}

	_, err := ParseAndGenerate(context.Background(), &req)
	require.Error(t, err)
}

func TestParseAndGenerate_EmptyPaths(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Empty", "version": "1.0.0"},
		"servers": [{"url": "https://example.com"}],
		"paths": {}
	}`)

	req := ImportRequest{
		SpecBody: spec,
		Mode:     ModeSingle,
	}

	_, err := ParseAndGenerate(context.Background(), &req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no operations")
}

func TestParseAndGenerate_NoServersNoOverride(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "No Servers", "version": "1.0.0"},
		"paths": {"/test": {"get": {"operationId": "test"}}}
	}`)

	req := ImportRequest{
		SpecBody: spec,
		Mode:     ModeSingle,
	}

	_, err := ParseAndGenerate(context.Background(), &req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no base URL")
}

func TestToModels(t *testing.T) {
	generated := []GeneratedTool{
		{
			ToolID:             "my_tool",
			Description:        "A tool",
			BaseURL:            "https://api.example.com",
			Transport:          "http",
			AuthType:           "bearer",
			InputSchemaJSON:    json.RawMessage(`{}`),
			OpenAPISpecURL:     "https://api.example.com/openapi.json",
			OpenAPIOperationID: "myOp",
		},
	}

	models := ToModels(generated, "t1", "secret_token")
	require.Len(t, models, 1)
	require.Equal(t, "my_tool", models[0].ToolID)
	require.Equal(t, "t1", models[0].TenantID)
	require.Equal(t, "secret_token", models[0].AuthValue)
	require.Equal(t, "openapi_import", models[0].Source)
	require.Equal(t, "https://api.example.com/openapi.json", models[0].OpenAPISpecURL)
	require.Equal(t, "myOp", models[0].OpenAPIOperationID)
}

func TestSanitizeToolID(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Pet Store", "pet_store"},
		{"my-api/v2", "my_api_v2"},
		{"/pets/{petId}", "pets_petid"},
		{"  ", "api"},
		{"UPPER", "upper"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.expected, sanitizeToolID(tc.input), "input: %q", tc.input)
	}
}
