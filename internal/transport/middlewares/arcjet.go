package middlewares

import (
	"log/slog"
	"net/http"

	"github.com/arcjet/arcjet-go"
	"github.com/labstack/echo/v5"
)

var arcjetExemptPaths = map[string]struct{}{
	"/favicon.ico":              {},
	"/health":                   {},
	"/ready":                    {},
	"/csrf":                     {},
	"/integrations/aws/sns/ses": {},
}

// Arcjet is a middleware that integrates the Arcjet client for request protection.
func Arcjet(client *arcjet.Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()

			if _, exempt := arcjetExemptPaths[req.URL.Path]; exempt {
				return next(c)
			}

			decision, err := client.Protect(
				req.Context(),
				req,
				arcjet.WithRequested(1),
			)
			if err != nil {
				// Arcjet fails open.
				slog.WarnContext(
					req.Context(),
					"Arcjet protection failed",
					"error", err,
					"method", req.Method,
					"path", req.URL.Path,
				)

				return next(c)
			}

			if decision.IsDenied() {
				status := http.StatusForbidden
				code := "request_denied"
				message := "The request was denied."

				if decision.Reason.IsRateLimit() {
					status = http.StatusTooManyRequests
					code = "rate_limit_exceeded"
					message = "Too many requests."
				} else if decision.IsSpoofedBot() {
					code = "spoofed_bot"
					message = "Automated request verification failed."
				}

				slog.WarnContext(
					req.Context(),
					"Arcjet denied request",
					"method", req.Method,
					"path", req.URL.Path,
					"status", status,
					"code", code,
					"reason", decision.Reason,
					"remote_addr", req.RemoteAddr,
					"x_forwarded_for", req.Header.Get("X-Forwarded-For"),
					"user_agent", req.UserAgent(),
				)

				return c.JSON(status, map[string]any{
					"error": map[string]any{
						"code":    code,
						"message": message,
					},
				})
			}

			return next(c)
		}
	}
}
