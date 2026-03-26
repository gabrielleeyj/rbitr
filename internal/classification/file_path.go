package classification

import (
	"path/filepath"
	"strings"
)

// FilePathResult holds all file paths detected in tool arguments along with
// sandbox validation results.
type FilePathResult struct {
	Paths          []string // all detected paths
	HasTraversal   bool     // true if any path contains ".." traversal
	TraversalPaths []string // paths that triggered traversal detection
	DeniedPaths    []string // paths outside tenant sandbox
}

// sandboxBase is the root directory for tenant file sandboxes.
const sandboxBase = "/data/tenants"

// DetectFilePaths recursively walks a parsed JSON value (map[string]any,
// []any, or string) and returns all string values that look like filesystem paths.
func DetectFilePaths(arguments any) []string {
	var paths []string
	detectFilePathsRecursive(arguments, &paths)
	return paths
}

func detectFilePathsRecursive(v any, paths *[]string) {
	switch val := v.(type) {
	case string:
		if IsFilePath(val) {
			*paths = append(*paths, val)
		}
	case map[string]any:
		for _, child := range val {
			detectFilePathsRecursive(child, paths)
		}
	case []any:
		for _, child := range val {
			detectFilePathsRecursive(child, paths)
		}
	}
}

// IsFilePath returns true if value looks like a filesystem path.
// Excludes URLs, email addresses, HTTP API paths, and other non-path strings.
func IsFilePath(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}

	// Exclude URLs and URI schemes.
	if isURIScheme(trimmed) {
		return false
	}

	// Exclude email-like strings.
	if strings.Contains(trimmed, "@") {
		return false
	}

	// Exclude short HTTP API paths (common pattern: /resource or /resource/id)
	// These are typically API endpoints, not filesystem paths.
	if isLikelyHTTPPath(trimmed) {
		return false
	}

	// Must start with a path-like prefix.
	if strings.HasPrefix(trimmed, "/") ||
		strings.HasPrefix(trimmed, "~/") ||
		strings.HasPrefix(trimmed, "./") ||
		strings.HasPrefix(trimmed, "../") {
		return true
	}

	// Windows absolute paths (C:\, D:\)
	if len(trimmed) >= 3 && trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/') {
		return true
	}

	return false
}

// ContainsTraversal returns true if the path contains ".." segments
// that could escape a directory boundary. Checks the raw path before
// cleaning, since filepath.Clean resolves traversal away.
func ContainsTraversal(p string) bool {
	// Normalize separators for cross-platform detection.
	normalized := strings.ReplaceAll(p, "\\", "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// ValidateSandbox checks that all paths are within the tenant sandbox root
// /data/tenants/{tenant_id}/. Returns a FilePathResult with traversal and
// denied path details.
func ValidateSandbox(paths []string, tenantID string) FilePathResult {
	root := SandboxRoot(tenantID)
	result := FilePathResult{
		Paths: paths,
	}

	for _, p := range paths {
		if ContainsTraversal(p) {
			result.HasTraversal = true
			result.TraversalPaths = append(result.TraversalPaths, p)
			continue
		}

		cleaned := filepath.Clean(p)
		// Path must be under the sandbox root.
		if !strings.HasPrefix(cleaned, root+"/") && cleaned != root {
			result.DeniedPaths = append(result.DeniedPaths, p)
		}
	}

	return result
}

// SandboxRoot returns the canonical sandbox root for a tenant.
func SandboxRoot(tenantID string) string {
	return filepath.Join(sandboxBase, tenantID)
}

// ValidateTenantID checks that a tenant ID is safe for use in filesystem paths.
// Rejects empty, traversal-containing, or slash-containing IDs.
func ValidateTenantID(tenantID string) bool {
	if tenantID == "" {
		return false
	}
	if strings.ContainsAny(tenantID, "/\\") {
		return false
	}
	if strings.Contains(tenantID, "..") {
		return false
	}
	if strings.ContainsRune(tenantID, 0) {
		return false
	}
	return true
}

func isURIScheme(s string) bool {
	schemes := []string{
		"http://", "https://", "ftp://", "ftps://",
		"s3://", "gs://", "ssh://", "git://",
		"file://", "data:", "mailto:",
	}
	lower := strings.ToLower(s)
	for _, scheme := range schemes {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
}

const (
	maxHTTPPathLength   = 50
	maxHTTPPathSegments = 3
)

// isLikelyHTTPPath returns true if the path looks like an HTTP API endpoint
// rather than a filesystem path. HTTP paths are short, have few segments,
// and don't contain filesystem indicators like file extensions or system directories.
func isLikelyHTTPPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return false
	}

	// Short paths with few segments are likely HTTP API paths
	if len(path) >= maxHTTPPathLength {
		return false
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) > maxHTTPPathSegments {
		return false
	}

	// Check if any segment contains filesystem indicators
	filesystemDirs := []string{"data", "tmp", "etc", "var", "opt", "home", "usr"}
	for _, seg := range segments {
		// Look for file extensions
		if strings.Contains(seg, ".") {
			return false
		}
		// Look for system directories
		for _, dir := range filesystemDirs {
			if seg == dir {
				return false
			}
		}
	}

	return true // Likely an HTTP API path
}
