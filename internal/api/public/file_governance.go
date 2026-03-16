package public

import (
	"log/slog"

	"github.com/gabrielleeyj/rbitr/internal/classification"
)

// checkFileAccess detects file paths in tool arguments and blocks path
// traversal or sandbox violations. Returns an error message if the request
// should be denied, or an empty string if access is allowed.
func (d *Dependencies) checkFileAccess(arguments any, tenantID string) string {
	paths := classification.DetectFilePaths(arguments)
	if len(paths) == 0 {
		return ""
	}

	result := classification.ValidateSandbox(paths, tenantID)

	if result.HasTraversal {
		slog.Warn("file path traversal blocked",
			"tenant_id", tenantID,
			"traversal_paths", result.TraversalPaths,
		)
		return "file path traversal detected"
	}

	if len(result.DeniedPaths) > 0 {
		slog.Warn("file sandbox violation blocked",
			"tenant_id", tenantID,
			"denied_paths", result.DeniedPaths,
		)
		return "file access outside tenant sandbox"
	}

	return ""
}
