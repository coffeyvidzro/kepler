package monitoring

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

var ignoredHTTPPaths = map[string]struct{}{
	"/health": {},
	"/ready":  {},
}

// NewRelic initializes a New Relic application when a license key is configured.
// Monitoring remains optional so local development does not require credentials.
func NewRelic(
	defaultAppName string,
	environment string,
	cfg config.NewRelicConfig,
) (*newrelic.Application, error) {
	if cfg.LicenseKey == "" {
		return nil, nil
	}

	labels := map[string]string{"service": strings.TrimSpace(defaultAppName)}
	if environment = strings.TrimSpace(environment); environment != "" {
		labels["environment"] = environment
	}

	return newrelic.NewApplication(
		newrelic.ConfigAppName(defaultAppName),
		newrelic.ConfigLicense(cfg.LicenseKey),
		newrelic.ConfigDistributedTracerEnabled(cfg.DistributedTracingEnabled),
		newrelic.ConfigAppLogEnabled(cfg.LogEnabled),
		newrelic.ConfigLabels(labels),
		newrelic.ConfigCodeLevelMetricsEnabled(true),
	)
}

// Shutdown flushes pending telemetry without forcing callers to special-case a disabled agent.
func Shutdown(app *newrelic.Application, timeout time.Duration) {
	if app != nil {
		app.Shutdown(timeout)
	}
}

// WrapHTTP records inbound HTTP requests and propagates the transaction through request context.
func WrapHTTP(app *newrelic.Application, next http.Handler) http.Handler {
	if app == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ignored := ignoredHTTPPaths[r.URL.Path]; ignored {
			next.ServeHTTP(w, r)
			return
		}

		txn := app.StartTransaction(r.Method + " unmatched")
		defer txn.End()

		txn.SetWebRequestHTTP(r)
		w = txn.SetWebResponse(w)
		r = newrelic.RequestWithTransactionContext(r, txn)
		next.ServeHTTP(w, r)
	})
}

// Transaction starts a background transaction and returns a derived context.
// Call the returned finish function exactly once with the operation result.
func Transaction(
	ctx context.Context,
	app *newrelic.Application,
	name string,
) (context.Context, func(error)) {
	if app == nil {
		return ctx, func(error) {}
	}

	txn := app.StartTransaction(name)
	return newrelic.NewContext(ctx, txn), func(err error) {
		if err != nil {
			txn.NoticeError(err)
		}
		txn.End()
	}
}
