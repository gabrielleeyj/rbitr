package aws

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	mptypes "github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// flushInterval is how often buffered usage records are flushed to AWS.
	flushInterval = 1 * time.Hour

	// maxBatchSize is the maximum number of UsageRecords per BatchMeterUsage call.
	maxBatchSize = 25

	// earlyFlushThreshold triggers an early flush when the buffer exceeds this size.
	earlyFlushThreshold = 20

	// meteringDimension is the AWS dimension name for governed actions.
	meteringDimension = "governed_actions"
)

// Reporter implements license.UsageReporter by buffering usage records in memory
// and flushing them to AWS Marketplace via BatchMeterUsage on a regular interval.
type Reporter struct {
	meteringClient MeteringClient
	productCode    string
	customerID     string

	mu     sync.Mutex
	buffer []bufferedRecord
	flush  chan struct{} // signals an early flush
}

type bufferedRecord struct {
	quantity  int64
	timestamp time.Time
}

var _ license.UsageReporter = (*Reporter)(nil)

// NewReporter creates a new AWS Marketplace usage reporter.
func NewReporter(meteringClient MeteringClient, productCode, customerID string) *Reporter {
	return &Reporter{
		meteringClient: meteringClient,
		productCode:    productCode,
		customerID:     customerID,
		flush:          make(chan struct{}, 1),
	}
}

// RecordUsage buffers a usage record for eventual flushing to AWS.
// This method is safe for concurrent use.
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

// doFlush drains the buffer and sends records to AWS via BatchMeterUsage.
func (r *Reporter) doFlush(ctx context.Context) {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	aggregated := r.aggregate(records)

	for i := 0; i < len(aggregated); i += maxBatchSize {
		end := min(i+maxBatchSize, len(aggregated))
		batch := aggregated[i:end]

		input := &marketplacemetering.BatchMeterUsageInput{
			ProductCode:  &r.productCode,
			UsageRecords: batch,
		}

		output, err := r.meteringClient.BatchMeterUsage(ctx, input)
		if err != nil {
			slog.Error("aws marketplace: BatchMeterUsage failed, re-buffering records",
				"error", err,
				"record_count", len(batch),
			)
			r.reBuffer(batch)
			continue
		}

		if len(output.UnprocessedRecords) > 0 {
			slog.Warn("aws marketplace: some records were not processed, re-buffering",
				"unprocessed_count", len(output.UnprocessedRecords),
				"processed_count", len(output.Results),
			)
			r.reBuffer(output.UnprocessedRecords)
		} else {
			slog.Info("aws marketplace: usage records flushed",
				"record_count", len(batch),
			)
		}
	}
}

// aggregate combines buffered records into UsageRecords grouped by hourly bucket.
func (r *Reporter) aggregate(records []bufferedRecord) []mptypes.UsageRecord {
	buckets := make(map[time.Time]int64)
	for _, rec := range records {
		buckets[rec.timestamp] += rec.quantity
	}

	dim := meteringDimension
	result := make([]mptypes.UsageRecord, 0, len(buckets))
	for ts, qty := range buckets {
		q := int32(min(qty, math.MaxInt32)) //nolint:gosec // clamped to MaxInt32
		result = append(result, mptypes.UsageRecord{
			CustomerIdentifier: &r.customerID,
			Dimension:          &dim,
			Timestamp:          &ts,
			Quantity:           &q,
		})
	}

	return result
}

// reBuffer puts unprocessed records back into the buffer for retry.
func (r *Reporter) reBuffer(records []mptypes.UsageRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rec := range records {
		var qty int64
		if rec.Quantity != nil {
			qty = int64(*rec.Quantity)
		}
		var ts time.Time
		if rec.Timestamp != nil {
			ts = *rec.Timestamp
		}
		r.buffer = append(r.buffer, bufferedRecord{
			quantity:  qty,
			timestamp: ts,
		})
	}
}

// PendingCount returns the number of buffered records awaiting flush.
// Useful for testing and diagnostics.
func (r *Reporter) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buffer)
}

// SetCustomerID updates the customer identifier for metering records.
func (r *Reporter) SetCustomerID(customerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.customerID = customerID
}

// Flush triggers an immediate flush of buffered records. Returns an error
// description if the flush encounters issues.
func (r *Reporter) Flush(ctx context.Context) error {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return nil
	}
	records := r.buffer
	r.buffer = nil
	r.mu.Unlock()

	aggregated := r.aggregate(records)

	var lastErr error
	for i := 0; i < len(aggregated); i += maxBatchSize {
		end := min(i+maxBatchSize, len(aggregated))
		batch := aggregated[i:end]

		input := &marketplacemetering.BatchMeterUsageInput{
			ProductCode:  &r.productCode,
			UsageRecords: batch,
		}

		output, err := r.meteringClient.BatchMeterUsage(ctx, input)
		if err != nil {
			r.reBuffer(batch)
			lastErr = fmt.Errorf("BatchMeterUsage: %w", err)
			continue
		}

		if len(output.UnprocessedRecords) > 0 {
			r.reBuffer(output.UnprocessedRecords)
			lastErr = fmt.Errorf("BatchMeterUsage: %d unprocessed records", len(output.UnprocessedRecords))
		}
	}

	return lastErr
}
