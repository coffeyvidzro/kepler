// Package errors provides structured application errors that can be safely
// translated to HTTP responses without exposing internal causes.
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	CodeBadRequest          = "BAD_REQUEST"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeForbidden           = "FORBIDDEN"
	CodeStepUpRequired      = "STEP_UP_REQUIRED"
	CodeNotFound            = "NOT_FOUND"
	CodeConflict            = "CONFLICT"
	CodePaymentRequired     = "PAYMENT_REQUIRED"
	CodePayloadTooLarge     = "PAYLOAD_TOO_LARGE"
	CodeUnprocessableEntity = "UNPROCESSABLE_ENTITY"
	CodeTooManyRequests     = "TOO_MANY_REQUESTS"
	CodeInternal            = "INTERNAL_ERROR"
	CodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
)

// AppError is a structured application error. Err is intended for logging and
// error inspection only; it is never serialized.
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Err     error  `json:"-"`
}

// New constructs an application error and applies safe defaults for missing or
// invalid fields.
func New(code, message string, status int, cause error) *AppError {
	if status < http.StatusBadRequest || status > 599 {
		status = http.StatusInternalServerError
	}
	code = normalizeCode(code)
	if code == "" {
		code = codeForStatus(status)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = http.StatusText(status)
	}
	if message == "" {
		message = "Request failed"
	}
	return &AppError{Code: code, Message: message, Status: status, Err: cause}
}

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	base := strings.TrimSpace(e.Code + ": " + e.Message)
	if e.Err == nil {
		return base
	}
	return fmt.Sprintf("%s: %v", base, e.Err)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is lets errors.Is compare AppError values by code and, when supplied on the
// target, HTTP status.
func (e *AppError) Is(target error) bool {
	other, ok := target.(*AppError)
	if !ok || e == nil || other == nil {
		return false
	}
	if other.Code != "" && !strings.EqualFold(e.Code, other.Code) {
		return false
	}
	if other.Status != 0 && e.Status != other.Status {
		return false
	}
	return other.Code != "" || other.Status != 0
}

// As returns the first AppError in err's chain.
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if !stderrors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}

// IsCode reports whether err contains an AppError with code.
func IsCode(err error, code string) bool {
	appErr, ok := As(err)
	return ok && strings.EqualFold(appErr.Code, normalizeCode(code))
}

// StatusOf returns the public HTTP status represented by err.
func StatusOf(err error) int {
	if appErr, ok := As(err); ok && appErr.Status >= 400 && appErr.Status <= 599 {
		return appErr.Status
	}
	return http.StatusInternalServerError
}

// CodeOf returns the public application code represented by err.
func CodeOf(err error) string {
	if appErr, ok := As(err); ok && strings.TrimSpace(appErr.Code) != "" {
		return appErr.Code
	}
	return CodeInternal
}

// MessageOf returns the public message represented by err. Unknown errors are
// intentionally mapped to a generic message.
func MessageOf(err error) string {
	if appErr, ok := As(err); ok && strings.TrimSpace(appErr.Message) != "" {
		return appErr.Message
	}
	return "An unexpected error occurred"
}

func NewNotFound(message string) *AppError {
	return New(CodeNotFound, message, http.StatusNotFound, nil)
}

func NewBadRequest(message string) *AppError {
	return New(CodeBadRequest, message, http.StatusBadRequest, nil)
}

func NewPaymentRequired(message string) *AppError {
	return New(CodePaymentRequired, message, http.StatusPaymentRequired, nil)
}

func NewPayloadTooLarge(message string) *AppError {
	return New(CodePayloadTooLarge, message, http.StatusRequestEntityTooLarge, nil)
}

func NewUnauthorized(message string) *AppError {
	return New(CodeUnauthorized, message, http.StatusUnauthorized, nil)
}

func NewForbidden(message string) *AppError {
	return New(CodeForbidden, message, http.StatusForbidden, nil)
}

func NewStepUpRequired(message string) *AppError {
	return New(CodeStepUpRequired, message, http.StatusForbidden, nil)
}

func NewConflict(message string) *AppError {
	return New(CodeConflict, message, http.StatusConflict, nil)
}

func NewUnprocessableEntity(message string) *AppError {
	return New(CodeUnprocessableEntity, message, http.StatusUnprocessableEntity, nil)
}

func NewTooManyRequests(message string) *AppError {
	return New(CodeTooManyRequests, message, http.StatusTooManyRequests, nil)
}

// TooManyRequests is retained for compatibility with existing call sites.
func TooManyRequests(message string) *AppError {
	return NewTooManyRequests(message)
}

func NewInternal(message string, err error) *AppError {
	return New(CodeInternal, message, http.StatusInternalServerError, err)
}

func NewServiceUnavailable(message string, err error) *AppError {
	return New(CodeServiceUnavailable, message, http.StatusServiceUnavailable, err)
}

func normalizeCode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	code = strings.NewReplacer("-", "_", " ", "_").Replace(code)
	return code
}

func codeForStatus(status int) string {
	if status == http.StatusInternalServerError {
		return CodeInternal
	}
	code := normalizeCode(http.StatusText(status))
	if code == "" {
		return CodeInternal
	}
	return code
}
