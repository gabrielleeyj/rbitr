package azure

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReporter_RecordUsageBuffers(t *testing.T) {
	mock := &mockMeteringClient{}
	r := NewReporter(mock, "sub-1", PlanPro)

	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}
	if err := r.RecordUsage(context.Background(), "tenant-2", "2026-04", 3); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	if r.PendingCount() != 2 {
		t.Errorf("PendingCount = %d, want 2", r.PendingCount())
	}

	if mock.batchCalls != 0 {
		t.Errorf("batchCalls = %d, want 0", mock.batchCalls)
	}
}

func TestReporter_FlushAggregates(t *testing.T) {
	mock := &mockMeteringClient{
		batchFunc: func(_ context.Context, events []UsageEvent) (*BatchUsageResponse, error) {
			results := make([]UsageEventResult, 0, len(events))
			for range events {
				results = append(results, UsageEventResult{Status: "Accepted"})
			}
			return &BatchUsageResponse{Result: results, Count: len(events)}, nil
		},
	}
	r := NewReporter(mock, "sub-1", PlanPro)

	for range 5 {
		if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 2); err != nil {
			t.Fatalf("RecordUsage error: %v", err)
		}
	}

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if mock.batchCalls != 1 {
		t.Errorf("batchCalls = %d, want 1", mock.batchCalls)
	}

	if r.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0 after flush", r.PendingCount())
	}

	// All records in the same hour bucket should aggregate to 1 event.
	if len(mock.lastBatchReq) != 1 {
		t.Errorf("events = %d, want 1 (aggregated)", len(mock.lastBatchReq))
	}

	// Total quantity should be 10 (5 * 2).
	if mock.lastBatchReq[0].Quantity != 10 {
		t.Errorf("quantity = %f, want 10", mock.lastBatchReq[0].Quantity)
	}
}

func TestReporter_FlushEmpty(t *testing.T) {
	mock := &mockMeteringClient{}
	r := NewReporter(mock, "sub-1", PlanPro)

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if mock.batchCalls != 0 {
		t.Errorf("batchCalls = %d, want 0 for empty flush", mock.batchCalls)
	}
}

func TestReporter_FlushErrorReBuffers(t *testing.T) {
	mock := &mockMeteringClient{
		batchFunc: func(_ context.Context, _ []UsageEvent) (*BatchUsageResponse, error) {
			return nil, errors.New("network error")
		},
	}
	r := NewReporter(mock, "sub-1", PlanPro)

	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	err := r.Flush(context.Background())
	if err == nil {
		t.Fatal("expected error from Flush")
	}

	if r.PendingCount() == 0 {
		t.Error("PendingCount should be > 0 after flush error (re-buffered)")
	}
}

func TestReporter_SetSubscription(t *testing.T) {
	mock := &mockMeteringClient{
		batchFunc: func(_ context.Context, events []UsageEvent) (*BatchUsageResponse, error) {
			if events[0].ResourceID != "sub-new" {
				t.Errorf("ResourceID = %q, want %q", events[0].ResourceID, "sub-new")
			}
			if events[0].PlanID != PlanEnterprise {
				t.Errorf("PlanID = %q, want %q", events[0].PlanID, PlanEnterprise)
			}
			return &BatchUsageResponse{
				Result: []UsageEventResult{{Status: "Accepted"}},
				Count:  1,
			}, nil
		},
	}
	r := NewReporter(mock, "sub-old", PlanPro)

	r.SetSubscription("sub-new", PlanEnterprise)

	if err := r.RecordUsage(context.Background(), "t1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
}

func TestReporter_StartAndStop(t *testing.T) {
	mock := &mockMeteringClient{
		batchFunc: func(_ context.Context, events []UsageEvent) (*BatchUsageResponse, error) {
			results := make([]UsageEventResult, 0, len(events))
			for range events {
				results = append(results, UsageEventResult{Status: "Accepted"})
			}
			return &BatchUsageResponse{Result: results, Count: len(events)}, nil
		},
	}
	r := NewReporter(mock, "sub-1", PlanPro)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	if err := r.RecordUsage(context.Background(), "tenant-1", "2026-04", 1); err != nil {
		t.Fatalf("RecordUsage error: %v", err)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	if mock.batchCalls == 0 {
		t.Error("expected at least 1 batch call from shutdown flush")
	}
}

func TestReporter_AggregateHourBoundaries(t *testing.T) {
	mock := &mockMeteringClient{}
	r := NewReporter(mock, "sub-1", PlanPro)

	now := time.Now().UTC().Truncate(time.Hour)
	records := []bufferedRecord{
		{quantity: 5, timestamp: now},
		{quantity: 3, timestamp: now},
		{quantity: 2, timestamp: now.Add(-time.Hour)},
	}

	events := r.aggregate(records)

	if len(events) != 2 {
		t.Errorf("events = %d, want 2 (two hourly buckets)", len(events))
	}
}
