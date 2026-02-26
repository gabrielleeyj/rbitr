package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/utils"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRESTExecute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		request       Request
		responseBody  string
		responseLimit int64
		status        int
		headers       map[string]string
		expectBody    string
		expectErr     bool
	}{
		{
			name:          "within limit",
			request:       Request{Method: http.MethodGet, URL: "http://example"},
			responseBody:  "ok",
			responseLimit: 64,
			status:        http.StatusAccepted,
			headers:       map[string]string{"X-Test": "value"},
			expectBody:    "ok",
		},
		{
			name:          "truncated",
			request:       Request{Method: http.MethodGet, URL: "http://example"},
			responseBody:  "0123456789",
			responseLimit: 5,
			status:        http.StatusOK,
			expectBody:    "01234",
		},
		{
			name:      "invalid url",
			request:   Request{Method: http.MethodGet, URL: "http://[::1"},
			expectErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					body := io.NopCloser(strings.NewReader(tc.responseBody))
					resp := &http.Response{
						StatusCode: tc.status,
						Body:       body,
						Header:     make(http.Header),
					}
					for key, value := range tc.headers {
						resp.Header.Set(key, value)
					}
					return resp, nil
				}),
			}

			rest := &REST{Client: client, ResponseLimit: tc.responseLimit}
			resp, err := rest.Execute(context.Background(), tc.request)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.status, resp.Status)
			require.Equal(t, tc.expectBody, string(resp.Body))
			if tc.headers != nil {
				for key, value := range tc.headers {
					require.Equal(t, value, resp.Headers[key])
				}
			}
		})
	}
}

func TestRESTExecuteForwardsHeaders(t *testing.T) {
	t.Parallel()

	var gotHeader string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotHeader = req.Header.Get("X-Test")
			body := io.NopCloser(strings.NewReader("ok"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	rest := &REST{Client: client, ResponseLimit: 64}
	_, err := rest.Execute(context.Background(), Request{
		Method:  http.MethodGet,
		URL:     "http://example",
		Headers: map[string]string{"X-Test": "value"},
	})
	require.NoError(t, err)
	require.Equal(t, "value", gotHeader)
}

func TestRESTExecuteWithNilHeaders(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader("ok"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	rest := &REST{Client: client, ResponseLimit: 64}
	resp, err := rest.Execute(context.Background(), Request{Method: http.MethodGet, URL: "http://example"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, utils.HashBody([]byte("ok")), resp.BodyHash)
}

func TestRESTExecuteBodyHash(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader("payload"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	rest := &REST{Client: client, ResponseLimit: 64}
	resp, err := rest.Execute(context.Background(), Request{Method: http.MethodGet, URL: "http://example"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.BodyHash)
}

func TestRESTExecuteTransportError(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}

	rest := &REST{Client: client, ResponseLimit: 64}
	_, err := rest.Execute(context.Background(), Request{Method: http.MethodGet, URL: "http://example"})
	require.Error(t, err)
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestRESTExecuteBodyReadError(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(errReader{})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	rest := &REST{Client: client, ResponseLimit: 64}
	_, err := rest.Execute(context.Background(), Request{Method: http.MethodGet, URL: "http://example"})
	require.Error(t, err)
}

func TestNewREST(t *testing.T) {
	t.Parallel()

	rest := NewREST(256)
	require.NotNil(t, rest.Client)
	require.Equal(t, int64(256), rest.ResponseLimit)
	require.Equal(t, 10*time.Second, rest.Client.Timeout)
}
