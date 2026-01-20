package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

func allowedHeaders() map[string]bool {
	return map[string]bool{
		"content-type": true,
		"accept":       true,
		"user-agent":   true,
		"x-request-id": true,
	}
}

func FilterHeaders(headers map[string]string) map[string]string {
	filtered := make(map[string]string)
	allowed := allowedHeaders()
	for key, value := range headers {
		lower := strings.ToLower(key)
		if allowed[lower] {
			filtered[lower] = value
		}
	}
	return filtered
}

type CanonicalRequest struct {
	TenantID       string
	AgentID        string
	ToolID         string
	Method         string
	Path           string
	Query          string
	Headers        map[string]string
	BodyHash       string
	IdempotencyKey string
}

func HashBody(body []byte) string {
	h := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(h[:])
}

func HashCanonical(req *CanonicalRequest) string {
	builder := strings.Builder{}
	builder.WriteString(req.TenantID)
	builder.WriteString("\n")
	builder.WriteString(req.AgentID)
	builder.WriteString("\n")
	builder.WriteString(req.ToolID)
	builder.WriteString("\n")
	builder.WriteString(strings.ToUpper(req.Method))
	builder.WriteString("\n")
	builder.WriteString(req.Path)
	builder.WriteString("\n")
	builder.WriteString(req.Query)
	builder.WriteString("\n")
	builder.WriteString(req.IdempotencyKey)
	builder.WriteString("\n")
	builder.WriteString(req.BodyHash)
	builder.WriteString("\n")

	keys := make([]string, 0, len(req.Headers))
	for key := range req.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(":")
		builder.WriteString(req.Headers[key])
		builder.WriteString("\n")
	}

	h := sha256.Sum256([]byte(builder.String()))
	return "sha256:" + hex.EncodeToString(h[:])
}
