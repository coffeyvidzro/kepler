package mnotify

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestResponseCodeAcceptsStringAndNumber(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"status":"success","code":2000,"message":"ok","summary":{}}`,
		`{"status":"success","code":"2000","message":"ok","summary":{}}`,
	} {
		var response SendResponse
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if got := response.Code.String(); got != "2000" {
			t.Fatalf("response code = %q, want 2000", got)
		}
	}
}

func TestAPIErrorSafeToFallbackClassifiesDefinitiveRejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *APIError
		want bool
	}{
		{name: "body rejection is definitive", err: &APIError{Definitive: true}, want: true},
		{name: "bad request is definitive", err: &APIError{StatusCode: http.StatusBadRequest}, want: true},
		{name: "unauthorized is definitive", err: &APIError{StatusCode: http.StatusUnauthorized}, want: true},
		{name: "rate limit is definitive", err: &APIError{StatusCode: http.StatusTooManyRequests}, want: true},
		{name: "server error is ambiguous", err: &APIError{StatusCode: http.StatusInternalServerError}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.SafeToFallback(); got != tt.want {
				t.Fatalf("SafeToFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}
