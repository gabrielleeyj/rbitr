package testhelpers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/mock"
)

func TestMakeRequestWithParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{
			name:   "single param",
			params: Params{Names: []string{"id"}, Values: []string{"123"}},
			want:   "123",
		},
		{
			name:   "missing value",
			params: Params{Names: []string{"id"}, Values: []string{}},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := MakeRequestWithParams(http.MethodGet, nil, tc.params)
			if got := ctx.Param("id"); got != tc.want {
				t.Fatalf("expected param %q got %q", tc.want, got)
			}
		})
	}
}

func TestMakeRequestHeaders(t *testing.T) {
	t.Parallel()

	ctx, req, _ := MakeRequest(http.MethodPost, map[string]string{"X-Test": "value"}, strings.NewReader("body"))
	if req.Header.Get("X-Test") != "value" {
		t.Fatalf("expected header to be set")
	}
	if ctx.Request().Method != http.MethodPost {
		t.Fatalf("expected method %s", http.MethodPost)
	}
}

func TestMakeBody(t *testing.T) {
	t.Parallel()

	body := MakeBody(map[string]string{"k": "v"})
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(body)
	if !strings.Contains(buf.String(), "\"k\":\"v\"") {
		t.Fatalf("expected json body, got %s", buf.String())
	}
}

func TestAddNewlineAndWrap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    []byte
		expected string
		fn       func([]byte) []byte
	}{
		{
			name:     "newline",
			input:    []byte("abc"),
			expected: "abc\n",
			fn:       AddNewline,
		},
		{
			name:     "wrap",
			input:    []byte("abc"),
			expected: "[abc]\n",
			fn:       WrapInBrackets,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(tc.fn(tc.input)); got != tc.expected {
				t.Fatalf("expected %q got %q", tc.expected, got)
			}
		})
	}
}

func TestDoRequestUsesJSONHeaders(t *testing.T) {
	t.Parallel()

	handler := func(c *echo.Context) error {
		if c.Request().Header.Get(echo.HeaderContentType) != echo.MIMEApplicationJSON {
			t.Fatalf("expected json content type")
		}
		return c.NoContent(http.StatusOK)
	}

	_, err := DoPOST(handler, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpectHTTPErrorHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		status     int
		message    string
		expectFunc func(*testing.T, error, int, string)
	}{
		{
			name:    "with message",
			err:     echo.NewHTTPError(http.StatusBadRequest, "bad"),
			status:  http.StatusBadRequest,
			message: "bad",
			expectFunc: func(t *testing.T, err error, status int, message string) {
				ExpectHTTPErrorWithMsg(t, err, status, message)
			},
		},
		{
			name:       "status only",
			err:        echo.NewHTTPError(http.StatusUnauthorized, "unauthorized"),
			status:     http.StatusUnauthorized,
			expectFunc: func(t *testing.T, err error, status int, _ string) { ExpectHTTPError(t, err, status) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.expectFunc(t, tc.err, tc.status, tc.message)
		})
	}
}

func TestAsJSON(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(`{"a":1,"b":"two"}`)
	out := AsJSON(input)
	if !strings.Contains(out, `"a":1`) || !strings.Contains(out, `"b":"two"`) {
		t.Fatalf("unexpected json output: %s", out)
	}
}

func TestDoPOSTWithForm(t *testing.T) {
	t.Parallel()

	handler := func(c *echo.Context) error {
		if !strings.HasPrefix(c.Request().Header.Get(echo.HeaderContentType), "multipart/form-data") {
			t.Fatalf("expected multipart content type")
		}
		value := c.FormValue("name")
		if value != "value" {
			t.Fatalf("expected form value, got %q", value)
		}
		return c.NoContent(http.StatusOK)
	}

	_, err := DoPOSTWithForm(handler, map[string]io.Reader{"name": strings.NewReader("value")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMakeErrorTests(t *testing.T) {
	t.Parallel()

	var got error
	cb := func(err error) *mock.Call {
		got = err
		return nil
	}

	handler := func(c *echo.Context) error {
		return echo.NewHTTPError(http.StatusBadRequest, "bad")
	}

	tc := MakeErrorTest("error", handler, cb, ErrOopsie, http.StatusBadRequest)
	tc.Test(t)
	if !errors.Is(got, ErrOopsie) {
		t.Fatalf("expected callback error to be ErrOopsie")
	}

	cb2 := func(err error) *mock.Call {
		got = err
		return nil
	}
	tc2 := MakeErrorTestWithCbs("error cbs", handler, []MockCb{cb2}, ErrOopsie, http.StatusBadRequest)
	tc2.Test(t)
	if !errors.Is(got, ErrOopsie) {
		t.Fatalf("expected callback error to be ErrOopsie")
	}
}
