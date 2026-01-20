package testhelpers

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
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
