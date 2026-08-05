package moolre

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrClientUnavailable = errors.New("moolre client is unavailable")
	ErrInvalidResponse   = errors.New("invalid Moolre response")
)

type APIError struct {
	StatusCode int
	Status     ResponseStatus
	Code       string
	Message    string
	Body       string
	Definitive bool
}

func (err *APIError) Error() string {
	if err == nil {
		return "moolre API error"
	}
	code := strings.TrimSpace(err.Code)
	message := strings.TrimSpace(err.Message)
	if code != "" || message != "" {
		return fmt.Sprintf("moolre API error: status %d code %q message %q", err.Status, code, message)
	}
	if strings.TrimSpace(err.Body) != "" {
		return fmt.Sprintf("moolre API returned HTTP status %d: %s", err.StatusCode, strings.TrimSpace(err.Body))
	}
	return fmt.Sprintf("moolre API returned HTTP status %d", err.StatusCode)
}

func (err *APIError) SafeToFallback() bool {
	if err == nil {
		return false
	}
	if err.Definitive {
		return true
	}
	if err.StatusCode < http.StatusBadRequest || err.StatusCode >= http.StatusInternalServerError {
		return false
	}
	switch err.StatusCode {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	default:
		return true
	}
}
