package verifymetrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsServeHTTP(t *testing.T) {
	metrics := New()
	metrics.Observe(" Dispatch ", "SUCCESS", 1500*time.Millisecond)
	metrics.Observe("dispatch", "error", 500*time.Millisecond)
	metrics.AddExpired(3)

	request := httptest.NewRequest(http.MethodGet, "/metrics/verify", nil)
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, expected := range []string{
		`dugble_verify_operations_total{operation="dispatch",outcome="error"} 1`,
		`dugble_verify_operations_total{operation="dispatch",outcome="success"} 1`,
		`dugble_verify_operation_duration_seconds_count{operation="dispatch"} 2`,
		`dugble_verify_operation_duration_seconds_sum{operation="dispatch"} 2`,
		`dugble_verify_expired_total 3`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics output missing %q:\n%s", expected, body)
		}
	}
}

func TestMetricsUseUnknownForBlankLabels(t *testing.T) {
	metrics := New()
	metrics.Observe("", "", 0)

	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics/verify", nil))

	if !strings.Contains(response.Body.String(), `operation="unknown",outcome="unknown"`) {
		t.Fatalf("metrics output did not normalize blank labels: %s", response.Body.String())
	}
}

func TestAddExpiredIgnoresNonPositiveValues(t *testing.T) {
	metrics := New()
	metrics.AddExpired(-1)
	metrics.AddExpired(0)

	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics/verify", nil))

	if !strings.Contains(response.Body.String(), "dugble_verify_expired_total 0") {
		t.Fatalf("unexpected expired counter: %s", response.Body.String())
	}
}
