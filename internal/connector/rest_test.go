package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRESTExecute(t *testing.T) {
	cases := []struct {
		name          string
		responseBody  string
		responseLimit int64
		expectedBody  string
	}{
		{
			name:          "within limit",
			responseBody:  "ok",
			responseLimit: 64,
			expectedBody:  "ok",
		},
		{
			name:          "truncated",
			responseBody:  "0123456789",
			responseLimit: 5,
			expectedBody:  "01234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Test", "value")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer srv.Close()

			rest := NewREST(tc.responseLimit)
			resp, err := rest.Execute(context.Background(), Request{
				Method: "GET",
				URL:    srv.URL,
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusAccepted, resp.Status)
			require.Equal(t, tc.expectedBody, string(resp.Body))
			require.Equal(t, "value", resp.Headers["X-Test"])
			require.NotEmpty(t, resp.BodyHash)
		})
	}
}
