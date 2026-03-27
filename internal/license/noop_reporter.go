package license

import "context"

// NoopReporter is a UsageReporter that does nothing. Used by the self-managed
// provider where usage metering is handled entirely by the local database.
type NoopReporter struct{}

var _ UsageReporter = (*NoopReporter)(nil)

// RecordUsage is a no-op for self-managed installations.
func (n *NoopReporter) RecordUsage(_ context.Context, _, _ string, _ int64) error {
	return nil
}

// Start blocks until ctx is cancelled.
func (n *NoopReporter) Start(ctx context.Context) {
	<-ctx.Done()
}
