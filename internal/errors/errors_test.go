package httperrors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestHTTPErrorHelpers(t *testing.T) {
	t.Parallel()

	type helper struct {
		name     string
		err      error
		fn       func(error) *echo.HTTPError
		expected int
		wantMsg  string
	}

	cases := []helper{
		{name: "unauthorized", err: errors.New("nope"), fn: Unauthorised, expected: http.StatusUnauthorized, wantMsg: "nope"},
		{name: "bad request", err: errors.New("bad"), fn: BadRequest, expected: http.StatusBadRequest, wantMsg: "bad"},
		{name: "conflict", err: errors.New("conflict"), fn: Conflict, expected: http.StatusConflict, wantMsg: "conflict"},
		{name: "forbidden", err: errors.New("forbidden"), fn: Forbidden, expected: http.StatusForbidden, wantMsg: "forbidden"},
		{name: "server error", err: errors.New("boom"), fn: ServerError, expected: http.StatusInternalServerError, wantMsg: "boom"},
		{name: "not found", err: errors.New("missing"), fn: NotFound, expected: http.StatusNotFound, wantMsg: "missing"},
		{name: "gone", err: errors.New("gone"), fn: Gone, expected: http.StatusGone, wantMsg: "gone"},
		{name: "unavailable", err: errors.New("down"), fn: Unavailable, expected: http.StatusServiceUnavailable, wantMsg: "down"},
		{name: "bad gateway", err: errors.New("bad gateway"), fn: BadGateway, expected: http.StatusBadGateway, wantMsg: "bad gateway"},
		{name: "gateway timeout", err: errors.New("timeout"), fn: GatewayTimeout, expected: http.StatusGatewayTimeout, wantMsg: "timeout"},
		{name: "nil error", err: nil, fn: BadRequest, expected: http.StatusBadRequest, wantMsg: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpErr := tc.fn(tc.err)
			if httpErr.Code != tc.expected {
				t.Fatalf("expected code %d got %d", tc.expected, httpErr.Code)
			}
			if httpErr.Message != tc.wantMsg {
				t.Fatalf("expected message %q got %q", tc.wantMsg, httpErr.Message)
			}
		})
	}
}

func TestHTTPErrorFormatters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fn       func(string, ...any) *echo.HTTPError
		format   string
		args     []any
		expected int
	}{
		{name: "unauthorizedf", fn: Unauthorisedf, format: "no %s", args: []any{"auth"}, expected: http.StatusUnauthorized},
		{name: "bad requestf", fn: BadRequestf, format: "bad %s", args: []any{"input"}, expected: http.StatusBadRequest},
		{name: "conflictf", fn: Conflictf, format: "conflict %s", args: []any{"state"}, expected: http.StatusConflict},
		{name: "forbiddenf", fn: Forbiddenf, format: "no %s", args: []any{"access"}, expected: http.StatusForbidden},
		{name: "serverf", fn: ServerErrorf, format: "boom %d", args: []any{5}, expected: http.StatusInternalServerError},
		{name: "not foundf", fn: NotFoundf, format: "missing %s", args: []any{"thing"}, expected: http.StatusNotFound},
		{name: "gonef", fn: Gonef, format: "gone %s", args: []any{"thing"}, expected: http.StatusGone},
		{name: "unavailablef", fn: Unavailablef, format: "down %s", args: []any{"service"}, expected: http.StatusServiceUnavailable},
		{name: "bad gatewayf", fn: BadGatewayf, format: "bad %s", args: []any{"gateway"}, expected: http.StatusBadGateway},
		{name: "gateway timeoutf", fn: GatewayTimeoutf, format: "timeout %s", args: []any{"gateway"}, expected: http.StatusGatewayTimeout},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpErr := tc.fn(tc.format, tc.args...)
			if httpErr.Code != tc.expected {
				t.Fatalf("expected code %d got %d", tc.expected, httpErr.Code)
			}
			if httpErr.Message == "" {
				t.Fatalf("expected non-empty message")
			}
		})
	}
}
