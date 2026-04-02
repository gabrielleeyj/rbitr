package gcp

import (
	"context"
	"errors"
	"testing"
	"time"

	servicecontrol "google.golang.org/api/servicecontrol/v2"
)

func TestReporter_RecordUsageBuffers(t *testing.T) {
	mock := &mockServiceControlClient{}
	r := NewReporter(mock, "my-service")

	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}
	if err := r.RecordUsage(context.Background(), "tenant-2", "2026-04", 3); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	if r.PendingCount() != 2 {
		t.Errorf("PendingCount = %d, want 2", r.PendingCount())
	}

	// No flush should have occurred.
	if mock.reportCalls != 0 {
		t.Errorf("reportCalls = %d, want 0", mock.reportCalls)
	}
}

func TestReporter_FlushAggregates(t *testing.T) {
	mock := &mockServiceControlClient{}
	r := NewReporter(mock, "my-service")

	// Record multiple usages in the same hour bucket.
	for range 5 {
		if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 2); err != nil {
			t.Fatalf("RecordUsage error: %v", err)
		}
	}

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if mock.reportCalls != 1 {
		t.Errorf("reportCalls = %d, want 1", mock.reportCalls)
	}

	if r.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0 after flush", r.PendingCount())
	}

	// Check that operations were aggregated (all same hour bucket = 1 operation).
	if mock.lastReportReq != nil && len(mock.lastReportReq.Operations) != 1 {
		t.Errorf("operations = %d, want 1 (aggregated)", len(mock.lastReportReq.Operations))
	}
}

func TestReporter_FlushEmpty(t *testing.T) {
	mock := &mockServiceControlClient{}
	r := NewReporter(mock, "my-service")

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if mock.reportCalls != 0 {
		t.Errorf("reportCalls = %d, want 0 for empty flush", mock.reportCalls)
	}
}

func TestReporter_FlushErrorReBuffers(t *testing.T) {
	mock := &mockServiceControlClient{
		reportFunc: func(_ string, _ *servicecontrol.ReportRequest) error {
			return errors.New("network error")
		},
	}
	r := NewReporter(mock, "my-service")

	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	err := r.Flush(context.Background())
	if err == nil {
		t.Fatal("expected error from Flush")
	}

	// Records should be re-buffered.
	if r.PendingCount() == 0 {
		t.Error("PendingCount should be > 0 after flush error (re-buffered)")
	}
}

func TestReporter_StartAndStop(t *testing.T) {
	mock := &mockServiceControlClient{}
	r := NewReporter(mock, "my-service")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Add a record so the final flush has something.
	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	// Final flush should have been attempted.
	if mock.reportCalls == 0 {
		t.Error("expected at least 1 report call from shutdown flush")
	}
}

func TestReporter_BuildOperationsAggregation(t *testing.T) {
	r := NewReporter(&mockServiceControlClient{}, "my-service")

	now := time.Now().UTC().Truncate(time.Hour)
	records := []bufferedRecord{
		{tenantID: "t1", quantity: 5, timestamp: now},
		{tenantID: "t2", quantity: 3, timestamp: now},
		{tenantID: "t1", quantity: 2, timestamp: now.Add(-time.Hour)},
	}

	ops := r.buildOperations(records)

	if len(ops) != 2 {
		t.Errorf("operations = %d, want 2 (two hourly buckets)", len(ops))
	}
}
