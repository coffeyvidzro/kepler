package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/labstack/echo/v5"
	"github.com/newrelic/go-agent/v3/newrelic"
)

var ignoredMonitoringPaths = map[string]struct{}{"/health": {}, "/ready": {}}

func NewRelic() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			txn := newrelic.FromContext(c.Request().Context())
			if txn != nil {
				path := strings.TrimSpace(c.RouteInfo().Path)
				if path == "" {
					path = "unmatched"
				}
				txn.SetName(c.Request().Method + " " + path)
				txn.AddAttribute("http.route", path)
			}
			err := next(c)
			if txn != nil {
				if response, ok := c.Response().(*echo.Response); ok {
					txn.AddAttribute("http.statusCode", response.Status)
				}
				if shouldReport(err) {
					txn.NoticeError(err)
				}
			}
			return err
		}
	}
}

func SentryErrors() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if _, ignored := ignoredMonitoringPaths[c.Request().URL.Path]; ignored || !shouldReport(err) {
				return err
			}
			hub := sentryecho.GetHubFromContext(c)
			if hub == nil {
				hub = sentry.CurrentHub()
			}
			hub.WithScope(func(scope *sentry.Scope) {
				route := strings.TrimSpace(c.RouteInfo().Path)
				if route == "" {
					route = "unmatched"
				}
				scope.SetTag("http.route", route)
				if requestID := strings.TrimSpace(c.Request().Header.Get(echo.HeaderXRequestID)); requestID != "" {
					scope.SetTag("request_id", requestID)
				}
				if txn := newrelic.FromContext(c.Request().Context()); txn != nil {
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

func shouldReport(err error) bool {
	if err == nil {
		return false
	}
	var httpError *echo.HTTPError
	if errors.As(err, &httpError) {
		return httpError.Code >= http.StatusInternalServerError
	}
	return true
}
