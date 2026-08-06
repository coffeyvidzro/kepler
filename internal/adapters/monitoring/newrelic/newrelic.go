package newrelic

import (
	"context"
	"net/http"
	"strings"
	"time"

	newrelicagent "github.com/newrelic/go-agent/v3/newrelic"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

var ignoredHTTPPaths = map[string]struct{}{
	"/health": {},
	"/ready":  {},
}

// New initializes a New Relic application when a license key is configured.
func New(
	defaultAppName string,
	environment string,
	configuration config.NewRelicConfig,
) (*newrelicagent.Application, error) {
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
	return newrelicagent.NewApplication(
		newrelicagent.ConfigAppName(defaultAppName),
		newrelicagent.ConfigLicense(configuration.LicenseKey),
		newrelicagent.ConfigDistributedTracerEnabled(configuration.DistributedTracingEnabled),
		newrelicagent.ConfigAppLogEnabled(configuration.LogEnabled),
		newrelicagent.ConfigLabels(labels),
		newrelicagent.ConfigCodeLevelMetricsEnabled(true),
	)
}

func Shutdown(application *newrelicagent.Application, timeout time.Duration) {
	if application != nil {
		application.Shutdown(timeout)
	}
}

func WrapHTTP(application *newrelicagent.Application, next http.Handler) http.Handler {
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
		request = newrelicagent.RequestWithTransactionContext(request, transaction)
		next.ServeHTTP(writer, request)
	})
}

func Transaction(
	ctx context.Context,
	application *newrelicagent.Application,
	name string,
) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if application == nil {
		return ctx, func(error) {}
	}
	transaction := application.StartTransaction(strings.TrimSpace(name))
	return newrelicagent.NewContext(ctx, transaction), func(err error) {
		if err != nil {
			transaction.NoticeError(err)
		}
		transaction.End()
	}
}
