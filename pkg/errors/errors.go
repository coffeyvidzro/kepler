package errors

import (
	"fmt"
	"net/http"
)

// AppError is a structured application error.
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}

	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewNotFound(message string) *AppError {
	return &AppError{
		Code:    "NOT_FOUND",
		Message: message,
		Status:  http.StatusNotFound,
	}
}

func NewBadRequest(message string) *AppError {
	return &AppError{
		Code:    "BAD_REQUEST",
		Message: message,
		Status:  http.StatusBadRequest,
	}
}

func NewPaymentRequired(message string) *AppError {
	return &AppError{
		Code:    "PAYMENT_REQUIRED",
		Message: message,
		Status:  http.StatusPaymentRequired,
	}
}

func NewPayloadTooLarge(message string) *AppError {
	return &AppError{
		Code:    "PAYLOAD_TOO_LARGE",
		Message: message,
		Status:  http.StatusRequestEntityTooLarge,
	}
}

func NewUnauthorized(message string) *AppError {
	return &AppError{
		Code:    "UNAUTHORIZED",
		Message: message,
		Status:  http.StatusUnauthorized,
	}
}

func NewForbidden(message string) *AppError {
	return &AppError{
		Code:    "FORBIDDEN",
		Message: message,
		Status:  http.StatusForbidden,
	}
}

func NewStepUpRequired(message string) *AppError {
	return &AppError{
		Code:    "STEP_UP_REQUIRED",
		Message: message,
		Status:  http.StatusForbidden,
	}
}

func NewConflict(message string) *AppError {
	return &AppError{
		Code:    "CONFLICT",
		Message: message,
		Status:  http.StatusConflict,
	}
}

func TooManyRequests(message string) *AppError {
	return &AppError{
		Code:    "TOO_MANY_REQUESTS",
		Message: message,
		Status:  http.StatusTooManyRequests,
	}
}

func NewInternal(message string, err error) *AppError {
	return &AppError{
		Code:    "INTERNAL_ERROR",
		Message: message,
		Status:  http.StatusInternalServerError,
		Err:     err,
	}
}

func NewServiceUnavailable(message string, err error) *AppError {
	return &AppError{
		Code:    "SERVICE_UNAVAILABLE",
		Message: message,
		Status:  http.StatusServiceUnavailable,
		Err:     err,
	}
}
