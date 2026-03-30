package aws_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	mptypes "github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awslicense "github.com/gabrielleeyj/rbitr/internal/license/aws"
)

func TestReporter_RecordUsage(t *testing.T) {
	t.Parallel()

	metClient := &mockMeteringClient{}
	reporter := awslicense.NewReporter(metClient, "prod-1", "cust-1")

	err := reporter.RecordUsage(context.Background(), "tenant-1", "2026-03", 1)
	assert.NoError(t, err)
	assert.Equal(t, 1, reporter.PendingCount())

	err = reporter.RecordUsage(context.Background(), "tenant-2", "2026-03", 5)
	assert.NoError(t, err)
	assert.Equal(t, 2, reporter.PendingCount())
}

func TestReporter_FlushSuccess(t *testing.T) {
	t.Parallel()

	metClient := &mockMeteringClient{}
	reporter := awslicense.NewReporter(metClient, "prod-1", "cust-1")

	for i := 0; i < 5; i++ {
		_ = reporter.RecordUsage(context.Background(), "t1", "2026-03", 1)
	}
	require.Equal(t, 5, reporter.PendingCount())

	err := reporter.Flush(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, reporter.PendingCount())
	assert.Equal(t, 1, metClient.batchMeterUsageCalls)

	// All records in the same hourly bucket should be aggregated
	require.NotNil(t, metClient.lastBatchInput)
	assert.Len(t, metClient.lastBatchInput.UsageRecords, 1) // 1 hourly bucket
	assert.Equal(t, int32(5), *metClient.lastBatchInput.UsageRecords[0].Quantity)
}

func TestReporter_FlushEmpty(t *testing.T) {
	t.Parallel()

	metClient := &mockMeteringClient{}
	reporter := awslicense.NewReporter(metClient, "prod-1", "cust-1")

	err := reporter.Flush(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, metClient.batchMeterUsageCalls)
}

func TestReporter_FlushError(t *testing.T) {
	t.Parallel()

	metClient := &mockMeteringClient{
		batchMeterUsageFunc: func(_ context.Context, _ *marketplacemetering.BatchMeterUsageInput, _ ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error) {
			return nil, errors.New("service unavailable")
		},
	}
	reporter := awslicense.NewReporter(metClient, "prod-1", "cust-1")

	_ = reporter.RecordUsage(context.Background(), "t1", "2026-03", 3)
	require.Equal(t, 1, reporter.PendingCount())

	err := reporter.Flush(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service unavailable")

	// Records should be re-buffered for retry
	assert.Equal(t, 1, reporter.PendingCount())
}

func TestReporter_UnprocessedRecords(t *testing.T) {
	t.Parallel()

	dim := "governed_actions"
	metClient := &mockMeteringClient{
		batchMeterUsageFunc: func(_ context.Context, input *marketplacemetering.BatchMeterUsageInput, _ ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error) {
			// Return first record as unprocessed
			return &marketplacemetering.BatchMeterUsageOutput{
				Results:            nil,
				UnprocessedRecords: []mptypes.UsageRecord{{Dimension: &dim, Quantity: ptr(int32(1))}},
			}, nil
		},
	}
	reporter := awslicense.NewReporter(metClient, "prod-1", "cust-1")

	_ = reporter.RecordUsage(context.Background(), "t1", "2026-03", 1)

	err := reporter.Flush(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unprocessed records")

	// Unprocessed records should be re-buffered
	assert.Equal(t, 1, reporter.PendingCount())
}

func TestReporter_SetCustomerID(t *testing.T) {
	t.Parallel()

	metClient := &mockMeteringClient{}
	reporter := awslicense.NewReporter(metClient, "prod-1", "initial")

	reporter.SetCustomerID("updated-cust")

	_ = reporter.RecordUsage(context.Background(), "t1", "2026-03", 1)
	err := reporter.Flush(context.Background())
	assert.NoError(t, err)

	require.NotNil(t, metClient.lastBatchInput)
	assert.Equal(t, "updated-cust", *metClient.lastBatchInput.UsageRecords[0].CustomerIdentifier)
}
