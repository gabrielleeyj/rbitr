package classification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFilePath(t *testing.T) {
	t.Parallel()

	positives := []string{
		"/etc/passwd",
		"/var/log/syslog",
		"~/config.json",
		"./local/file.txt",
		"../secret/data",
		"/data/tenants/t1/upload.csv",
		"C:\\Users\\admin\\file.txt",
		"D:/documents/report.pdf",
	}

	for _, p := range positives {
		assert.True(t, IsFilePath(p), "expected true for %q", p)
	}

	negatives := []string{
		"",
		"   ",
		"just-a-string",
		"some text with spaces",
		"https://example.com/path/to/file",
		"http://localhost:8080/api",
		"ftp://server/file",
		"s3://bucket/key",
		"gs://bucket/object",
		"user@host.com",
		"data:text/plain;base64,abc",
		"mailto:user@example.com",
		"192.168.1.1",
		"@user/repo",
		"file://localhost/etc/passwd",
	}

	for _, n := range negatives {
		assert.False(t, IsFilePath(n), "expected false for %q", n)
	}
}

func TestDetectFilePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string that is a path",
			input:    "/etc/passwd",
			expected: []string{"/etc/passwd"},
		},
		{
			name:     "string that is not a path",
			input:    "hello world",
			expected: nil,
		},
		{
			name: "flat map with one path",
			input: map[string]any{
				"file": "/tmp/data.csv",
				"name": "report",
			},
			expected: []string{"/tmp/data.csv"},
		},
		{
			name: "nested map with paths",
			input: map[string]any{
				"config": map[string]any{
					"input":  "/data/tenants/t1/input.csv",
					"output": "/data/tenants/t1/output.json",
				},
				"label": "process",
			},
			expected: []string{"/data/tenants/t1/input.csv", "/data/tenants/t1/output.json"},
		},
		{
			name: "array with paths",
			input: map[string]any{
				"files": []any{"/tmp/a.txt", "/tmp/b.txt", "not-a-path"},
			},
			expected: []string{"/tmp/a.txt", "/tmp/b.txt"},
		},
		{
			name: "deeply nested",
			input: map[string]any{
				"level1": map[string]any{
					"level2": []any{
						map[string]any{
							"path": "../secret.key",
						},
					},
				},
			},
			expected: []string{"../secret.key"},
		},
		{
			name: "mixed types no paths",
			input: map[string]any{
				"count":  42,
				"active": true,
				"url":    "https://example.com/file",
			},
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DetectFilePaths(tc.input)
			if tc.expected == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tc.expected, got)
			}
		})
	}
}

func TestContainsTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{"/data/tenants/t1/file.txt", false},
		{"./safe/file.txt", false},
		{"/safe", false},
		{"../secret", true},
		{"/data/tenants/t1/../../etc/passwd", true},
		{"foo/../bar", true},
		{"/data/tenants/../../../etc/shadow", true},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, ContainsTraversal(tc.path), "path: %q", tc.path)
	}
}

func TestValidateSandbox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		paths            []string
		tenantID         string
		wantTraversal    bool
		wantDeniedCount  int
		wantTraversalCnt int
	}{
		{
			name:     "all paths within sandbox",
			paths:    []string{"/data/tenants/t1/file.csv", "/data/tenants/t1/subdir/report.json"},
			tenantID: "t1",
		},
		{
			name:            "path outside sandbox",
			paths:           []string{"/etc/passwd", "/data/tenants/t1/ok.txt"},
			tenantID:        "t1",
			wantDeniedCount: 1,
		},
		{
			name:             "traversal attempt",
			paths:            []string{"/data/tenants/t1/../../etc/passwd"},
			tenantID:         "t1",
			wantTraversal:    true,
			wantTraversalCnt: 1,
		},
		{
			name:            "different tenant sandbox",
			paths:           []string{"/data/tenants/t2/secret.txt"},
			tenantID:        "t1",
			wantDeniedCount: 1,
		},
		{
			name:     "empty paths",
			paths:    []string{},
			tenantID: "t1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ValidateSandbox(tc.paths, tc.tenantID)
			assert.Equal(t, tc.paths, result.Paths)
			assert.Equal(t, tc.wantTraversal, result.HasTraversal)
			assert.Len(t, result.DeniedPaths, tc.wantDeniedCount)
			assert.Len(t, result.TraversalPaths, tc.wantTraversalCnt)
		})
	}
}

func TestSandboxRoot(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/data/tenants/t_demo", SandboxRoot("t_demo"))
}

func TestValidateTenantID(t *testing.T) {
	t.Parallel()

	valids := []string{"t1", "tenant_demo", "t-123"}
	for _, id := range valids {
		assert.True(t, ValidateTenantID(id), "expected valid: %q", id)
	}

	invalids := []string{"", "..", "t1/../../etc", "t1\\bad", "t\x001"}
	for _, id := range invalids {
		assert.False(t, ValidateTenantID(id), "expected invalid: %q", id)
	}
}
