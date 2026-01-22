package connector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
)

func TestMockConnectorExpectations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	req := Request{Method: "GET", URL: "http://example"}
	resp := Response{Status: 200}

	t.Run("Execute", func(t *testing.T) {
		conn := NewMockConnector(t)
		conn.EXPECT().Execute(ctx, req).Return(resp, nil)
		_, _ = conn.Execute(ctx, req)
	})

	t.Run("ExecuteRun", func(t *testing.T) {
		conn := NewMockConnector(t)
		conn.EXPECT().Execute(ctx, mock.Anything).Run(func(context.Context, Request) {}).Return(resp, nil)
		_, _ = conn.Execute(ctx, req)
	})
}
