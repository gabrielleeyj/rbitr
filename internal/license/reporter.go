package license

import "context"

// UsageReporter abstracts external usage reporting.
// Self-managed uses a no-op (local DB metering is sufficient).
// Marketplace providers batch and report to external metering APIs.
type UsageReporter interface {
	// RecordUsage records a single governed action for eventual external reporting.
	// tenantID and period are passed for local tracking; implementations may batch
	// and flush externally.
	RecordUsage(ctx context.Context, tenantID, period string, quantity int64) error

	// Start begins the background flush loop. Blocks until ctx is cancelled.
	Start(ctx context.Context)
}
