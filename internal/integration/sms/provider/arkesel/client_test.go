package arkesel

import (
	"net/http"
	"testing"
)

func TestAPIErrorSafeToFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{name: "bad request is safe", statusCode: http.StatusBadRequest, want: true},
		{name: "rate limit is safe", statusCode: http.StatusTooManyRequests, want: true},
		{name: "unauthorized is unsafe", statusCode: http.StatusUnauthorized, want: false},
		{name: "forbidden is unsafe", statusCode: http.StatusForbidden, want: false},
		{name: "server error is unsafe", statusCode: http.StatusInternalServerError, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := &APIError{StatusCode: tt.statusCode}
			if got := err.SafeToFallback(); got != tt.want {
				t.Fatalf("SafeToFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}
