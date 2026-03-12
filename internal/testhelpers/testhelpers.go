package testhelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var ErrOopsie = errors.New("oops")

func defaultJSONHeaders() map[string]string {
	return map[string]string{
		echo.HeaderContentType: echo.MIMEApplicationJSON,
	}
}

// NoopHandler is a noop echo handler.
func NoopHandler(c *echo.Context) error {
	return c.NoContent(http.StatusOK)
}

// ExpectHTTPError asserts that err is a echo HTTPError w/ statusCode.
func ExpectHTTPError(t *testing.T, err error, statusCode int) {
	require.Error(t, err)
	var e *echo.HTTPError
	require.True(t, errors.As(err, &e))
	assert.Equal(t, statusCode, e.Code)
}

// ExpectHTTPErrorWithMsg asserts that err is a echo HTTPError w/ statusCode and msg.
func ExpectHTTPErrorWithMsg(t *testing.T, err error, statusCode int, msg string) {
	require.Error(t, err)
	var e *echo.HTTPError
	require.True(t, errors.As(err, &e))
	require.Equal(t, statusCode, e.Code)
	require.Equal(t, msg, e.Message)
}

// DoGET makes a GET request.
func DoGET(h echo.HandlerFunc) (*httptest.ResponseRecorder, error) {
	return DoRequest(h, http.MethodGet, nil)
}

// DoGETWithParams makes a GET request with URL params.
func DoGETWithParams(h echo.HandlerFunc, params Params) (*httptest.ResponseRecorder, error) {
	return DoRequestWithParams(h, http.MethodGet, nil, params)
}

// DoGETWithBody makes a GET request with body.
func DoGETWithBody(h echo.HandlerFunc, body io.Reader) (*httptest.ResponseRecorder, error) {
	return DoRequest(h, http.MethodGet, body)
}

// DoPOST makes a POST request.
func DoPOST(h echo.HandlerFunc, body io.Reader) (*httptest.ResponseRecorder, error) {
	return DoRequest(h, http.MethodPost, body)
}

// DoPOSTWithParams makes a POST request with URL params.
func DoPOSTWithParams(h echo.HandlerFunc, body io.Reader, params Params) (*httptest.ResponseRecorder, error) {
	return DoRequestWithParams(h, http.MethodPost, body, params)
}

// DoPOSTWithForm makes a POST request w/ multipart form body.
func DoPOSTWithForm(h echo.HandlerFunc, values map[string]io.Reader) (*httptest.ResponseRecorder, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	var err error

	for key, r := range values {
		var fw io.Writer
		var closer io.Closer

		if x, ok := r.(io.Closer); ok {
			closer = x
		}

		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return nil, err
			}
		} else {
			if fw, err = w.CreateFormField(key); err != nil {
				return nil, err
			}
		}

		if _, err = io.Copy(fw, r); err != nil {
			return nil, err
		}
		if closer != nil {
			_ = closer.Close()
		}
	}

	w.Close()

	ctx, _, rec := MakeRequest(http.MethodPost, map[string]string{
		echo.HeaderContentType: w.FormDataContentType(),
	}, &b)

	return rec, h(ctx)
}

// DoPUTWithParams makes a PUT request with URL params.
func DoPUTWithParams(h echo.HandlerFunc, body io.Reader, params Params) (*httptest.ResponseRecorder, error) {
	return DoRequestWithParams(h, http.MethodPut, body, params)
}

// DoPUT makes a PUT request.
func DoPUT(h echo.HandlerFunc, body io.Reader) (*httptest.ResponseRecorder, error) {
	return DoRequest(h, http.MethodPut, body)
}

// DoRequest makes a request with method and body.
func DoRequest(h echo.HandlerFunc, method string, body io.Reader) (*httptest.ResponseRecorder, error) {
	ctx, _, rec := MakeRequest(method, defaultJSONHeaders(), body)
	return rec, h(ctx)
}

// DoRequestWithParams makes a request with method, body and URL params.
func DoRequestWithParams(
	h echo.HandlerFunc,
	method string,
	body io.Reader,
	params Params,
) (*httptest.ResponseRecorder, error) {
	ctx, _, rec := MakeRequestWithParams(method, body, params)
	return rec, h(ctx)
}

func MakeRequestWithParams(
	method string,
	body io.Reader,
	params Params,
) (*echo.Context, *http.Request, *httptest.ResponseRecorder) {
	ctx, req, rec := MakeRequest(method, defaultJSONHeaders(), body)
	pathValues := make(echo.PathValues, 0, len(params.Names))
	for i, name := range params.Names {
		if i >= len(params.Values) {
			break
		}
		pathValues = append(pathValues, echo.PathValue{Name: name, Value: params.Values[i]})
	}
	if len(pathValues) > 0 {
		ctx.SetPathValues(pathValues)
	}
	return ctx, req, rec
}

type Params struct {
	Names  []string
	Values []string
}

// MakeRequest returns a echo context set w/ a request w/ method, headers and body and a response recorder.
func MakeRequest(
	method string,
	headers map[string]string,
	body io.Reader,
) (*echo.Context, *http.Request, *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequestWithContext(context.Background(), method, "/", body)

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	return ctx, req, rec
}

// MakeBody encodes v to JSON and returns it as a io.Reader.
func MakeBody(v any) io.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewBuffer(data)
}

// AsJSON decodes JSON and encodes back to JSON.
//
// See https://github.com/golang/protobuf/issues/1351#issuecomment-896163454 for why we need this.
func AsJSON(v io.Reader) string {
	decoder := json.NewDecoder(v)
	var dst map[string]any
	_ = decoder.Decode(&dst)
	b, _ := json.Marshal(dst)
	return string(b)
}

// AddNewline adds a \n char to a byte array.
func AddNewline(data []byte) []byte {
	return append(data, []byte("\n")...)
}

// WrapInBrackets adds [] around a byte array and then a \n char at the end.
func WrapInBrackets(data []byte) []byte {
	ob := []byte("[")
	ob = append(ob, data...)
	ob = append(ob, []byte("]")...)
	return AddNewline(ob)
}

type MockCb func(err error) *mock.Call

type TestCase struct {
	Name string
	Test func(t *testing.T)
}

func MakeErrorTest(name string,
	h echo.HandlerFunc,
	c MockCb,
	retErr error, expStatusCode int,
) TestCase {
	return TestCase{
		Name: name,
		Test: func(t *testing.T) {
			c(retErr)
			_, err := DoPOST(h, nil)
			ExpectHTTPError(t, err, expStatusCode)
		},
	}
}

func MakeErrorTestWithCbs(name string,
	h echo.HandlerFunc,
	c []MockCb,
	retErr error, expStatusCode int,
) TestCase {
	return TestCase{
		Name: name,
		Test: func(t *testing.T) {
			for _, cb := range c {
				cb(retErr)
			}
			_, err := DoPOST(h, nil)
			ExpectHTTPError(t, err, expStatusCode)
		},
	}
}
