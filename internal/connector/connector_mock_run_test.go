package connector

import (
	"context"
	"testing"
)

func TestMockConnectorRunAndReturn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	req := Request{Method: "GET", URL: "http://example"}

	conn := NewMockConnector(t)
	call := conn.EXPECT().Execute(ctx, req)
	call.Run(func(context.Context, Request) {})
	call.RunAndReturn(func(context.Context, Request) (Response, error) {
		return Response{Status: 201}, nil
	})
	call.Once()
	resp, err := conn.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 201 {
		t.Fatalf("expected status 201 got %d", resp.Status)
	}
}
