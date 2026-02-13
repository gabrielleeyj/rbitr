package mcp

import (
	"bytes"
	"encoding/json"
)

// ExtractRequestID attempts to extract just the "id" field from raw JSON-RPC request bytes.
// This is used to preserve request correlation in error responses even when validation fails.
// Uses streaming decoder to handle trailing data gracefully - only parses the first JSON object.
// Returns nil if ID cannot be extracted.
func ExtractRequestID(data []byte) *RequestID {
	// Use decoder to parse only the first JSON object (handles trailing data)
	decoder := json.NewDecoder(bytes.NewReader(data))

	var partial struct {
		ID json.RawMessage `json:"id"`
	}

	if err := decoder.Decode(&partial); err != nil {
		return nil
	}

	// No ID field present
	if len(partial.ID) == 0 {
		return nil
	}

	// Try to unmarshal the ID
	var id RequestID
	if err := id.UnmarshalJSON(partial.ID); err != nil {
		// ID was present but invalid - return nil rather than propagating parse errors
		return nil
	}

	return &id
}
