package connector

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type REST struct {
	Client        *http.Client
	ResponseLimit int64
}

func NewREST(responseLimit int64) *REST {
	const requestTimeout = 10 * time.Second

	return &REST{
		Client: &http.Client{
			Timeout: requestTimeout,
		},
		ResponseLimit: responseLimit,
	}
}

// Execute forwards the http request from the original source to the destination.
func (r *REST) Execute(ctx context.Context, req Request) (Response, error) {
	if err := validateOutboundURL(req.URL); err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return Response{}, err
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := r.Client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, r.ResponseLimit)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Response{}, err
	}

	respHeaders := make(map[string]string)
	for key := range resp.Header {
		respHeaders[key] = resp.Header.Get(key)
	}

	return Response{
		Status:   resp.StatusCode,
		Headers:  respHeaders,
		Body:     body,
		BodyHash: utils.HashBody(body),
	}, nil
}
