package arkesel

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrClientUnavailable = errors.New("Arkesel client is unavailable")
	ErrInvalidResponse   = errors.New("invalid Arkesel response")
)

type APIError struct {
	StatusCode int
	Body       string
}

func (err *APIError) Error() string {
	if err == nil {
		return "Arkesel API error"
	}
	if strings.TrimSpace(err.Body) == "" {
		return fmt.Sprintf("Arkesel API error: status code %d", err.StatusCode)
	}
	return fmt.Sprintf("Arkesel API error: status code %d: %s", err.StatusCode, strings.TrimSpace(err.Body))
}

func (err *APIError) SafeToFallback() bool {
	if err == nil {
		return false
	}
	switch err.StatusCode {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusConflict,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusTeapot,
		http.StatusMisdirectedRequest,
		http.StatusUnprocessableEntity,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusTooManyRequests,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons:
		return true
	default:
		return false
	}
}
