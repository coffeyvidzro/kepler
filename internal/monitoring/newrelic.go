// Package monitoring is retained as a compatibility bridge. New code should
// import internal/platform/monitoring.
package monitoring

import (
	"context"
	"net/http"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	platformmonitoring "github.com/coffeyvidzro/dugble/server/internal/platform/monitoring"
)

func NewRelic(name, environment string, configuration config.NewRelicConfig) (*newrelic.Application, error) {
	return platformmonitoring.NewRelic(name, environment, configuration)
}

func Shutdown(application *newrelic.Application, timeout time.Duration) {
	platformmonitoring.Shutdown(application, timeout)
}

func WrapHTTP(application *newrelic.Application, next http.Handler) http.Handler {
	return platformmonitoring.WrapHTTP(application, next)
}

func Transaction(
	ctx context.Context,
	application *newrelic.Application,
	name string,
) (context.Context, func(error)) {
	return platformmonitoring.Transaction(ctx, application, name)
}
