package monitoring

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newrelic/go-agent/v3/newrelic"
)

func TestWrapHTTPIgnoresHealthChecks(t *testing.T) {
	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName("test"),
		newrelic.ConfigLicense("1234567890123456789012345678901234567890"),
		newrelic.ConfigEnabled(false),
	)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}

	var hasTransaction bool
	handler := WrapHTTP(app, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hasTransaction = newrelic.FromContext(r.Context()) != nil
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	if hasTransaction {
		t.Fatal("expected /health to bypass New Relic transaction creation")
	}
}

func TestWrapHTTPAddsTransactionContext(t *testing.T) {
	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName("test"),
		newrelic.ConfigLicense("1234567890123456789012345678901234567890"),
		newrelic.ConfigEnabled(false),
	)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}

	var hasTransaction bool
	handler := WrapHTTP(app, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hasTransaction = newrelic.FromContext(r.Context()) != nil
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))

	if !hasTransaction {
		t.Fatal("expected non-health request to include New Relic transaction context")
	}
}
