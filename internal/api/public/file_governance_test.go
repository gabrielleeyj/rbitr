package public

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gabrielleeyj/rbitr/internal/config"
)

func TestCheckFileAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     any
		tenantID string
		want     string
	}{
		{
			name:     "no file paths",
			args:     map[string]any{"query": "SELECT 1"},
			tenantID: "t1",
			want:     "",
		},
		{
			name:     "path within sandbox",
			args:     map[string]any{"file": "/data/tenants/t1/upload.csv"},
			tenantID: "t1",
			want:     "",
		},
		{
			name:     "path outside sandbox",
			args:     map[string]any{"file": "/etc/passwd"},
			tenantID: "t1",
			want:     "file access outside tenant sandbox",
		},
		{
			name:     "traversal attempt",
			args:     map[string]any{"file": "/data/tenants/t1/../../etc/passwd"},
			tenantID: "t1",
			want:     "file path traversal detected",
		},
		{
			name:     "different tenant sandbox",
			args:     map[string]any{"file": "/data/tenants/t2/secret.txt"},
			tenantID: "t1",
			want:     "file access outside tenant sandbox",
		},
		{
			name:     "nil arguments",
			args:     nil,
			tenantID: "t1",
			want:     "",
		},
		{
			name:     "nested traversal",
			args:     map[string]any{"config": map[string]any{"path": "../../../etc/shadow"}},
			tenantID: "t1",
			want:     "file path traversal detected",
		},
		{
			name:     "HTTP API path /refund - not detected as file",
			args:     map[string]any{"path": "/refund", "method": "POST"},
			tenantID: "t1",
			want:     "",
		},
		{
			name:     "HTTP API path /customers/123 - not detected as file",
			args:     map[string]any{"path": "/customers/123"},
			tenantID: "t1",
			want:     "",
		},
		{
			name:     "HTTP API path /api/v1/users - not detected as file",
			args:     map[string]any{"url": "/api/v1/users"},
			tenantID: "t1",
			want:     "",
		},
	}

	deps := &Dependencies{
		Config: config.Config{FeatureFileGovernance: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := deps.checkFileAccess(tc.args, tc.tenantID)
			assert.Equal(t, tc.want, got)
		})
	}
}
