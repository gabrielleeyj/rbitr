package httperrors

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

func Unauthorised(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusUnauthorized, messageFromErr(err))
}

func Unauthorisedf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusUnauthorized, fmt.Sprintf(format, a...))
}

func BadRequest(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadRequest, messageFromErr(err))
}

func BadRequestf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadRequest,
		fmt.Sprintf(format, a...))
}

func Conflict(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusConflict, messageFromErr(err))
}

func Conflictf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusConflict,
		fmt.Sprintf(format, a...))
}

func Forbidden(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusForbidden, messageFromErr(err))
}

func Forbiddenf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusForbidden,
		fmt.Sprintf(format, a...))
}

func ServerError(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusInternalServerError, messageFromErr(err))
}

func ServerErrorf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusInternalServerError,
		fmt.Sprintf(format, a...))
}

func NotFound(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusNotFound, messageFromErr(err))
}

func NotFoundf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusNotFound,
		fmt.Sprintf(format, a...))
}

func Gone(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusGone, messageFromErr(err))
}

func Gonef(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusGone,
		fmt.Sprintf(format, a...))
}

func Unavailable(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusServiceUnavailable, messageFromErr(err))
}

func Unavailablef(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusServiceUnavailable,
		fmt.Sprintf(format, a...))
}

func BadGateway(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadGateway, messageFromErr(err))
}

func BadGatewayf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadGateway,
		fmt.Sprintf(format, a...))
}

func GatewayTimeout(err error) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusGatewayTimeout, messageFromErr(err))
}

func messageFromErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func GatewayTimeoutf(format string, a ...any) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusGatewayTimeout,
		fmt.Sprintf(format, a...))
}
