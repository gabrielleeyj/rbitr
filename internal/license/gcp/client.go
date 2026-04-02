package gcp

import (
	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
	servicecontrol "google.golang.org/api/servicecontrol/v2"
)

// ProcurementClient abstracts the GCP Cloud Commerce Partner Procurement API.
// The real implementation wraps *procurement.Service.
type ProcurementClient interface {
	// ListEntitlements lists entitlements for the given provider.
	// parent is "providers/{provider_id}".
	ListEntitlements(parent string) (*procurement.ListEntitlementsResponse, error)

	// ApproveEntitlement approves an entitlement activation request.
	ApproveEntitlement(name string) error
}

// ServiceControlClient abstracts the GCP Service Control API v2.
// The real implementation wraps *servicecontrol.Service.
type ServiceControlClient interface {
	// Report sends usage operations to the Service Control API.
	Report(serviceName string, req *servicecontrol.ReportRequest) error
}

// sdkProcurementClient wraps the real GCP Procurement SDK.
type sdkProcurementClient struct {
	svc *procurement.Service
}

// NewSDKProcurementClient creates a ProcurementClient wrapping the real GCP SDK.
func NewSDKProcurementClient(svc *procurement.Service) ProcurementClient {
	return &sdkProcurementClient{svc: svc}
}

func (c *sdkProcurementClient) ListEntitlements(parent string) (*procurement.ListEntitlementsResponse, error) {
	return c.svc.Providers.Entitlements.List(parent).Do()
}

func (c *sdkProcurementClient) ApproveEntitlement(name string) error {
	_, err := c.svc.Providers.Entitlements.Approve(name, &procurement.ApproveEntitlementRequest{}).Do()
	return err
}

// sdkServiceControlClient wraps the real GCP Service Control SDK.
type sdkServiceControlClient struct {
	svc *servicecontrol.Service
}

// NewSDKServiceControlClient creates a ServiceControlClient wrapping the real GCP SDK.
func NewSDKServiceControlClient(svc *servicecontrol.Service) ServiceControlClient {
	return &sdkServiceControlClient{svc: svc}
}

func (c *sdkServiceControlClient) Report(serviceName string, req *servicecontrol.ReportRequest) error {
	_, err := c.svc.Services.Report(serviceName, req).Do()
	return err
}
