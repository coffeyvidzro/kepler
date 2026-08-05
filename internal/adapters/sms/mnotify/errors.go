package mnotify

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrClientUnavailable = errors.New("mNotify client is unavailable")
	ErrInvalidResponse   = errors.New("invalid mNotify response")
)

type APIError struct {
	StatusCode int
	Status     string
	Code       ResponseCode
	Message    string
	Body       string
	Definitive bool
}

func (err *APIError) Error() string {
	if err == nil {
		return "mNotify API error"
	}
	code := err.Code.String()
	message := strings.TrimSpace(err.Message)
	status := strings.TrimSpace(err.Status)
	if code != "" || message != "" || status != "" {
		return fmt.Sprintf("mNotify API error: status %q code %q message %q", status, code, message)
	}
	if strings.TrimSpace(err.Body) != "" {
		return fmt.Sprintf("mNotify API returned status %d: %s", err.StatusCode, strings.TrimSpace(err.Body))
	}
	return fmt.Sprintf("mNotify API returned status %d", err.StatusCode)
}

func (err *APIError) SafeToFallback() bool {
	if err == nil {
		return false
	}
	if err.Definitive {
		return true
	}
	return err.StatusCode >= http.StatusBadRequest && err.StatusCode < http.StatusInternalServerError
}
