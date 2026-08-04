package middlewares

import (
	"errors"
	"net/http"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/labstack/echo/v5"
	"github.com/newrelic/go-agent/v3/newrelic"
)

var sentryIgnoredPaths = map[string]struct{}{
	"/health": {},
	"/ready":  {},
}

// SentryErrors captures unexpected handler errors that do not panic.
// Panic capture is handled by sentryecho.New with Repanic enabled.
func SentryErrors() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if !shouldReportToSentry(c, err) {
				return err
			}

			hub := sentryecho.GetHubFromContext(c)
			if hub == nil {
				hub = sentry.CurrentHub()
			}

			hub.WithScope(func(scope *sentry.Scope) {
				request := c.Request()
				route := strings.TrimSpace(c.RouteInfo().Path)
				if route == "" {
					route = "unmatched"
				}

				scope.SetTag("http.route", route)
				if requestID := strings.TrimSpace(request.Header.Get("X-Request-ID")); requestID != "" {
					scope.SetTag("request_id", requestID)
				}

				if txn := newrelic.FromContext(request.Context()); txn != nil {
					metadata := txn.GetTraceMetadata()
					if metadata.TraceID != "" {
						scope.SetTag("new_relic.trace_id", metadata.TraceID)
					}
					if metadata.SpanID != "" {
						scope.SetTag("new_relic.span_id", metadata.SpanID)
					}
				}

				hub.CaptureException(err)
			})

			return err
		}
	}
}

func shouldReportToSentry(c *echo.Context, err error) bool {
	if err == nil {
		return false
	}
	if _, ignored := sentryIgnoredPaths[c.Request().URL.Path]; ignored {
		return false
	}

	var httpError *echo.HTTPError
	if errors.As(err, &httpError) {
		return httpError.Code >= http.StatusInternalServerError
	}

	return true
}
