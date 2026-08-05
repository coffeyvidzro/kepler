package celcom

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrClientUnavailable = errors.New("Celcom client is unavailable")
	ErrInvalidResponse   = errors.New("invalid Celcom response")
)

type APIError struct {
	HTTPStatus  int
	Code        int
	Description string
	Body        string
}

func (err *APIError) Error() string {
	if err == nil {
		return "Celcom API error"
	}
	if strings.TrimSpace(err.Description) != "" {
		return fmt.Sprintf("Celcom API error: code %d: %s", err.Code, strings.TrimSpace(err.Description))
	}
	if strings.TrimSpace(err.Body) != "" {
		return fmt.Sprintf("Celcom API error: status %d: %s", err.HTTPStatus, strings.TrimSpace(err.Body))
	}
	if err.Code != 0 {
		return fmt.Sprintf("Celcom API error: code %d", err.Code)
	}
	return fmt.Sprintf("Celcom API error: status %d", err.HTTPStatus)
}

func (err *APIError) SafeToFallback() bool {
	if err == nil {
		return false
	}
	if err.Code != 0 {
		switch err.Code {
		case 1001, 1002, 1003, 1004, 1006, 1008, 1009, 1010, 4091, 4092, 4093:
			return true
		default:
			return false
		}
	}
	return err.HTTPStatus >= http.StatusBadRequest &&
		err.HTTPStatus < http.StatusInternalServerError &&
		err.HTTPStatus != http.StatusUnauthorized &&
		err.HTTPStatus != http.StatusForbidden
}
