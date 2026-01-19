package connector

import "context"

type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

type Response struct {
	Status   int
	Headers  map[string]string
	Body     []byte
	BodyHash string
}

type Connector interface {
	Execute(ctx context.Context, req Request) (Response, error)
}
