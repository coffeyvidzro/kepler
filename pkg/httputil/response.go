// Package httputil contains the shared JSON response contract for Echo HTTP
// handlers.
package httputil

import (
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

// Response is the standard JSON envelope for API responses.
type Response struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *ErrorObj `json:"error,omitempty"`
}

// ErrorObj describes a public API error.
type ErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON sends a successful JSON response with status.
func JSON(c *echo.Context, status int, data any) error {
	if status == http.StatusNoContent {
		return c.NoContent(status)
	}
	return c.JSON(status, Response{Success: true, Data: data})
}

func OK(c *echo.Context, data any) error {
	return JSON(c, http.StatusOK, data)
}

func Created(c *echo.Context, data any) error {
	return JSON(c, http.StatusCreated, data)
}

func Accepted(c *echo.Context, data any) error {
	return JSON(c, http.StatusAccepted, data)
}

func NoContent(c *echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

// Partial sends committed data together with a public error description.
func Partial(c *echo.Context, status int, data any, err error) error {
	if status < http.StatusBadRequest || status > 599 {
		status = http.StatusInternalServerError
	}
	_, publicError := describeError(err)
	return c.JSON(status, Response{Success: false, Data: data, Error: publicError})
}

// Error sends a safe error response. Internal causes are retained in the error
// chain for logging but never included in the JSON body.
func Error(c *echo.Context, err error) error {
	status, publicError := describeError(err)
	return c.JSON(status, Response{Success: false, Error: publicError})
}

func describeError(err error) (int, *ErrorObj) {
	if appErr, ok := apperrors.As(err); ok {
		return appErr.Status, &ErrorObj{Code: appErr.Code, Message: appErr.Message}
	}

	var httpErr *echo.HTTPError
	if stderrors.As(err, &httpErr) {
		status := httpErr.Code
		if status < http.StatusBadRequest || status > 599 {
			status = http.StatusInternalServerError
		}
		message := http.StatusText(status)
		if status < http.StatusInternalServerError && strings.TrimSpace(httpErr.Message) != "" {
			message = strings.TrimSpace(httpErr.Message)
		}
		return status, &ErrorObj{Code: codeForStatus(status), Message: message}
	}

	return http.StatusInternalServerError, &ErrorObj{
		Code:    apperrors.CodeInternal,
		Message: "An unexpected error occurred",
	}
}

func codeForStatus(status int) string {
	if status == http.StatusInternalServerError {
		return apperrors.CodeInternal
	}
	code := strings.TrimSpace(strings.ToUpper(http.StatusText(status)))
	code = strings.NewReplacer("-", "_", " ", "_").Replace(code)
	if code == "" {
		return apperrors.CodeInternal
	}
	return code
}
