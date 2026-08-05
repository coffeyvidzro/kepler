package httptransport

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

// Response is the standard HTTP JSON envelope.
type Response struct {
	Success bool         `json:"success"`
	Data    any          `json:"data,omitempty"`
	Error   *ErrorObject `json:"error,omitempty"`
}

// ErrorObject is the public representation of an application error.
type ErrorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(c *echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{Success: true, Data: data})
}

func Created(c *echo.Context, data any) error {
	return c.JSON(http.StatusCreated, Response{Success: true, Data: data})
}

func Accepted(c *echo.Context, data any) error {
	return c.JSON(http.StatusAccepted, Response{Success: true, Data: data})
}

func NoContent(c *echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func Partial(c *echo.Context, status int, data any, err error) error {
	object := errorObject(err)
	return c.JSON(status, Response{Success: false, Data: data, Error: object})
}

func Error(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	var applicationError *apperrors.AppError
	if errors.As(err, &applicationError) {
		status = applicationError.Status
	}
	return c.JSON(status, Response{Success: false, Error: errorObject(err)})
}

func errorObject(err error) *ErrorObject {
	object := &ErrorObject{Code: "INTERNAL_ERROR", Message: "An unexpected error occurred"}
	var applicationError *apperrors.AppError
	if errors.As(err, &applicationError) {
		object.Code = applicationError.Code
		object.Message = applicationError.Message
	}
	return object
}
