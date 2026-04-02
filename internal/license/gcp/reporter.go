package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	servicecontrol "google.golang.org/api/servicecontrol/v2"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// flushInterval is how often buffered usage records are flushed to GCP.
	flushInterval = 1 * time.Hour

	// maxBatchSize is the maximum number of operations per Report call.
	maxBatchSize = 100

	// earlyFlushThreshold triggers an early flush when the buffer exceeds this size.
	earlyFlushThreshold = 80

	// operationName is the operation identifier used in Service Control reports.
	operationName = "rbitr.governed_action"
)

// Reporter implements license.UsageReporter by buffering usage records in memory
// and flushing them to GCP Service Control API on a regular interval.
type Reporter struct {
	scClient    ServiceControlClient
	serviceName string

	mu     sync.Mutex
	buffer []bufferedRecord
	flush  chan struct{}
}

type bufferedRecord struct {
	tenantID  string
	quantity  int64
	timestamp time.Time
}

var _ license.UsageReporter = (*Reporter)(nil)

// NewReporter creates a new GCP Marketplace usage reporter.
func NewReporter(scClient ServiceControlClient, serviceName string) *Reporter {
	return &Reporter{
		scClient:    scClient,
		serviceName: serviceName,
		flush:       make(chan struct{}, 1),
	}
}

// RecordUsage buffers a usage record for eventual flushing to GCP.
// This method is safe for concurrent use.
func (r *Reporter) RecordUsage(_ context.Context, tenantID, _ string, quantity int64) error {
	r.mu.Lock()
	r.buffer = append(r.buffer, bufferedRecord{
		tenantID:  tenantID,
		quantity:  quantity,
		timestamp: time.Now().UTC().Truncate(time.Hour),
	})
	shouldFlush := len(r.buffer) >= earlyFlushThreshold
	r.mu.Unlock()

	if shouldFlush {
		select {
		case r.flush <- struct{}{}:
		default:
		}
	}

	return nil
}

// Start begins the background flush loop. Blocks until ctx is cancelled.
// On shutdown, performs a final flush attempt.
func (r *Reporter) Start(ctx context.Context) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.doFlush(context.Background())
			return
		case <-ticker.C:
			r.doFlush(ctx)
		case <-r.flush:
			r.doFlush(ctx)
		}
	}
}

// doFlush drains the buffer and sends records to GCP via Service Control Report.
func (r *Reporter) doFlush(_ context.Context) {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	operations := r.buildOperations(records)

	for i := 0; i < len(operations); i += maxBatchSize {
		end := min(i+maxBatchSize, len(operations))
		batch := operations[i:end]

		req := &servicecontrol.ReportRequest{
			Operations: batch,
		}

		if err := r.scClient.Report(r.serviceName, req); err != nil {
			slog.Error("gcp marketplace: Service Control Report failed, re-buffering records",
				"error", err,
				"record_count", len(batch),
			)
			r.reBuffer(records[i:end])
			continue
		}

		slog.Info("gcp marketplace: usage records flushed",
			"record_count", len(batch),
		)
	}
}

// buildOperations converts buffered records into Service Control operations.
// Records are aggregated by hourly bucket.
func (r *Reporter) buildOperations(records []bufferedRecord) []*servicecontrol.AttributeContext {
	type bucketKey struct {
		hour time.Time
	}

	buckets := make(map[bucketKey]int64)
	for _, rec := range records {
		key := bucketKey{hour: rec.timestamp}
		buckets[key] += rec.quantity
	}

	ops := make([]*servicecontrol.AttributeContext, 0, len(buckets))
	for key, qty := range buckets {
		ops = append(ops, &servicecontrol.AttributeContext{
			Api: &servicecontrol.Api{
				Operation: operationName,
				Service:   r.serviceName,
			},
			Request: &servicecontrol.Request{
				Time: key.hour.Format(time.RFC3339),
				Size: qty,
			},
		})
	}

	return ops
}

// reBuffer puts unprocessed records back into the buffer for retry.
func (r *Reporter) reBuffer(records []bufferedRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer = append(r.buffer, records...)
}

// PendingCount returns the number of buffered records awaiting flush.
func (r *Reporter) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buffer)
}

// Flush triggers an immediate flush of buffered records.
func (r *Reporter) Flush(_ context.Context) error {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return nil
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	operations := r.buildOperations(records)

	var lastErr error
	for i := 0; i < len(operations); i += maxBatchSize {
		end := min(i+maxBatchSize, len(operations))
		batch := operations[i:end]

		req := &servicecontrol.ReportRequest{
			Operations: batch,
		}

		if err := r.scClient.Report(r.serviceName, req); err != nil {
			r.reBuffer(records[i:end])
			lastErr = fmt.Errorf("service control report: %w", err)
		}
	}

	return lastErr
}
