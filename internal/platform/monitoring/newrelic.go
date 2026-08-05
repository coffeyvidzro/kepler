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
func NewRelic(
	defaultAppName string,
	environment string,
	configuration config.NewRelicConfig,
) (*newrelic.Application, error) {
	configuration.LicenseKey = strings.TrimSpace(configuration.LicenseKey)
	if configuration.LicenseKey == "" {
		return nil, nil
	}
	defaultAppName = strings.TrimSpace(defaultAppName)
	if defaultAppName == "" {
		defaultAppName = "dugble"
	}

	labels := map[string]string{"service": defaultAppName}
	if environment = strings.TrimSpace(environment); environment != "" {
		labels["environment"] = environment
	}
	return newrelic.NewApplication(
		newrelic.ConfigAppName(defaultAppName),
		newrelic.ConfigLicense(configuration.LicenseKey),
		newrelic.ConfigDistributedTracerEnabled(configuration.DistributedTracingEnabled),
		newrelic.ConfigAppLogEnabled(configuration.LogEnabled),
		newrelic.ConfigLabels(labels),
		newrelic.ConfigCodeLevelMetricsEnabled(true),
	)
}

func Shutdown(application *newrelic.Application, timeout time.Duration) {
	if application != nil {
		application.Shutdown(timeout)
	}
}

func WrapHTTP(application *newrelic.Application, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	if application == nil {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ignored := ignoredHTTPPaths[request.URL.Path]; ignored {
			next.ServeHTTP(writer, request)
			return
		}
		transaction := application.StartTransaction(request.Method + " unmatched")
		defer transaction.End()
		transaction.SetWebRequestHTTP(request)
		writer = transaction.SetWebResponse(writer)
		request = newrelic.RequestWithTransactionContext(request, transaction)
		next.ServeHTTP(writer, request)
	})
}

func Transaction(
	ctx context.Context,
	application *newrelic.Application,
	name string,
) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if application == nil {
		return ctx, func(error) {}
	}
	transaction := application.StartTransaction(strings.TrimSpace(name))
	return newrelic.NewContext(ctx, transaction), func(err error) {
		if err != nil {
			transaction.NoticeError(err)
		}
		transaction.End()
	}
}
