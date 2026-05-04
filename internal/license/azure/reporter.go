package azure

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// flushInterval is how often buffered usage records are flushed to Azure.
	flushInterval = 1 * time.Hour

	// maxBatchSize is the maximum number of usage events per batch call.
	maxBatchSize = 25

	// earlyFlushThreshold triggers an early flush when the buffer exceeds this size.
	earlyFlushThreshold = 20

	// meteringDimension is the Azure dimension name for governed actions.
	meteringDimension = "governed_actions"

	// statusAccepted is the response status returned when Azure accepts a usage event.
	statusAccepted = "Accepted"
)

// Reporter implements license.UsageReporter by buffering usage records in memory
// and flushing them to Azure Marketplace Metering API on a regular interval.
type Reporter struct {
	meteringClient MeteringClient
	subscriptionID string
	planID         string

	mu     sync.Mutex
	buffer []bufferedRecord
	flush  chan struct{}
}

type bufferedRecord struct {
	quantity  int64
	timestamp time.Time
}

var _ license.UsageReporter = (*Reporter)(nil)

// NewReporter creates a new Azure Marketplace usage reporter.
func NewReporter(meteringClient MeteringClient, subscriptionID, planID string) *Reporter {
	return &Reporter{
		meteringClient: meteringClient,
		subscriptionID: subscriptionID,
		planID:         planID,
		flush:          make(chan struct{}, 1),
	}
}

// RecordUsage buffers a usage record for eventual flushing to Azure.
func (r *Reporter) RecordUsage(_ context.Context, _, _ string, quantity int64) error {
	r.mu.Lock()
	r.buffer = append(r.buffer, bufferedRecord{
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

// doFlush drains the buffer and sends records to Azure via batch metering.
func (r *Reporter) doFlush(ctx context.Context) {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	events := r.aggregate(records)

	for i := 0; i < len(events); i += maxBatchSize {
		end := min(i+maxBatchSize, len(events))
		batch := events[i:end]

		resp, err := r.meteringClient.BatchUsageEvent(ctx, batch)
		if err != nil {
			slog.Error("azure marketplace: batch metering failed, re-buffering records",
				"error", err,
				"record_count", len(batch),
			)
			r.reBuffer(batch)
			continue
		}

		failCount := 0
		for _, result := range resp.Result {
			if result.Status != statusAccepted {
				failCount++
			}
		}

		if failCount > 0 {
			slog.Warn("azure marketplace: some usage events were not accepted",
				"failed_count", failCount,
				"total_count", len(batch),
			)
		} else {
			slog.Info("azure marketplace: usage records flushed",
				"record_count", len(batch),
			)
		}
	}
}

// aggregate combines buffered records into UsageEvents grouped by hourly bucket.
func (r *Reporter) aggregate(records []bufferedRecord) []UsageEvent {
	buckets := make(map[time.Time]float64)
	for _, rec := range records {
		buckets[rec.timestamp] += float64(rec.quantity)
	}

	r.mu.Lock()
	subID := r.subscriptionID
	planID := r.planID
	r.mu.Unlock()

	result := make([]UsageEvent, 0, len(buckets))
	for ts, qty := range buckets {
		result = append(result, UsageEvent{
			ResourceID:    subID,
			Quantity:      qty,
			Dimension:     meteringDimension,
			EffectiveTime: ts,
			PlanID:        planID,
		})
	}

	return result
}

// reBuffer puts failed events back into the buffer for retry.
func (r *Reporter) reBuffer(events []UsageEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ev := range events {
		r.buffer = append(r.buffer, bufferedRecord{
			quantity:  int64(ev.Quantity),
			timestamp: ev.EffectiveTime,
		})
	}
}

// PendingCount returns the number of buffered records awaiting flush.
func (r *Reporter) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buffer)
}

// SetSubscription updates the subscription ID and plan for metering records.
func (r *Reporter) SetSubscription(subscriptionID, planID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptionID = subscriptionID
	r.planID = planID
}

// Flush triggers an immediate flush of buffered records.
func (r *Reporter) Flush(ctx context.Context) error {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return nil
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	events := r.aggregate(records)

	var lastErr error
	for i := 0; i < len(events); i += maxBatchSize {
		end := min(i+maxBatchSize, len(events))
		batch := events[i:end]

		_, err := r.meteringClient.BatchUsageEvent(ctx, batch)
		if err != nil {
			r.reBuffer(batch)
			lastErr = fmt.Errorf("batch usage event: %w", err)
		}
	}

	return lastErr
}
