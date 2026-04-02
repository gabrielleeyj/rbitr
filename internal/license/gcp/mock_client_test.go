package gcp

import (
	"sync"

	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
	servicecontrol "google.golang.org/api/servicecontrol/v2"
)

// mockProcurementClient is a test double for ProcurementClient.
type mockProcurementClient struct {
	mu               sync.Mutex
	listFunc         func(parent string) (*procurement.ListEntitlementsResponse, error)
	approveFunc      func(name string) error
	listCallCount    int
	approveCallCount int
	lastApprovedName string
}

func (m *mockProcurementClient) ListEntitlements(parent string) (*procurement.ListEntitlementsResponse, error) {
	m.mu.Lock()
	m.listCallCount++
	m.mu.Unlock()

	if m.listFunc != nil {
		return m.listFunc(parent)
	}
	return &procurement.ListEntitlementsResponse{}, nil
}

func (m *mockProcurementClient) ApproveEntitlement(name string) error {
	m.mu.Lock()
	m.approveCallCount++
	m.lastApprovedName = name
	m.mu.Unlock()

	if m.approveFunc != nil {
		return m.approveFunc(name)
	}
	return nil
}

// mockServiceControlClient is a test double for ServiceControlClient.
type mockServiceControlClient struct {
	mu            sync.Mutex
	reportFunc    func(serviceName string, req *servicecontrol.ReportRequest) error
	reportCalls   int
	lastReportReq *servicecontrol.ReportRequest
}

func (m *mockServiceControlClient) Report(serviceName string, req *servicecontrol.ReportRequest) error {
	m.mu.Lock()
	m.reportCalls++
	m.lastReportReq = req
	m.mu.Unlock()

	if m.reportFunc != nil {
		return m.reportFunc(serviceName, req)
	}
	return nil
}
