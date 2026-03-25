package admin

import (
	"encoding/json"
	"errors"
	"regexp"
)

var toolIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,63}$`)

var validAuthTypes = map[string]bool{
	"none":                       true,
	"bearer":                     true,
	"api_key":                    true,
	"oauth2_client_credentials":  true,
}

var validTransports = map[string]bool{
	"http": true,
	"mcp":  true,
}

func validateToolID(id string) error {
	if !toolIDPattern.MatchString(id) {
		return errors.New("tool_id must be 3-64 alphanumeric characters, underscores, or hyphens, starting with alphanumeric")
	}
	return nil
}

func validateAuthType(authType string) error {
	if !validAuthTypes[authType] {
		return errors.New("auth_type must be one of: none, bearer, api_key, oauth2_client_credentials")
	}
	return nil
}

func validateTransport(transport string) error {
	if !validTransports[transport] {
		return errors.New("transport must be one of: http, mcp")
	}
	return nil
}

func validateInputSchemaJSON(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return errors.New("input_schema_json must be a valid JSON object")
	}
	return nil
}
