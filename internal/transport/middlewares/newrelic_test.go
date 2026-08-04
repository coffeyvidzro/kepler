package middlewares

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestShouldNoticeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "client HTTP error", err: echo.NewHTTPError(http.StatusBadRequest, "bad request"), want: false},
		{name: "server HTTP error", err: echo.NewHTTPError(http.StatusInternalServerError, "boom"), want: true},
		{name: "wrapped server HTTP error", err: errors.Join(errors.New("handler failed"), echo.NewHTTPError(http.StatusServiceUnavailable, "unavailable")), want: true},
		{name: "plain error", err: errors.New("boom"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNoticeError(tt.err); got != tt.want {
				t.Fatalf("shouldNoticeError() = %t, want %t", got, tt.want)
			}
		})
	}
}
