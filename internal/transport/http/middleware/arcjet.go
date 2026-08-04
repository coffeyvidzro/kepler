package middleware

import (
	"log/slog"
	"net/http"

	"github.com/arcjet/arcjet-go"
	"github.com/labstack/echo/v5"
)

var arcjetExemptPaths = map[string]struct{}{
	"/favicon.ico": {}, "/health": {}, "/ready": {}, "/csrf": {},
	"/integrations/aws/sns/ses": {},
	"/integrations/sms/arkesel": {}, "/integrations/sms/celcom": {}, "/integrations/sms/mnotify": {},
}

func Arcjet(client *arcjet.Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if client == nil {
				return next(c)
			}
			request := c.Request()
			if _, exempt := arcjetExemptPaths[request.URL.Path]; exempt {
				return next(c)
			}
			decision, err := client.Protect(request.Context(), request, arcjet.WithRequested(1))
			if err != nil {
				slog.WarnContext(request.Context(), "Arcjet protection failed", "error", err, "method", request.Method, "path", request.URL.Path)
				return next(c)
			}
			if !decision.IsDenied() {
				return next(c)
			}
			status, code, message := http.StatusForbidden, "request_denied", "The request was denied."
			if decision.Reason.IsRateLimit() {
				status, code, message = http.StatusTooManyRequests, "rate_limit_exceeded", "Too many requests."
			} else if decision.IsSpoofedBot() {
				code, message = "spoofed_bot", "Automated request verification failed."
			}
			slog.WarnContext(request.Context(), "Arcjet denied request", "method", request.Method, "path", request.URL.Path, "status", status, "code", code, "reason", decision.Reason)
			return c.JSON(status, map[string]any{"error": map[string]any{"code": code, "message": message}})
		}
	}
}
