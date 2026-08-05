package middleware

import (
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
)

func AuditRequestContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		requestID := strings.TrimSpace(request.Header.Get(echo.HeaderXRequestID))
		if requestID == "" {
			requestID = strings.TrimSpace(c.Response().Header().Get(echo.HeaderXRequestID))
		}
		metadata := audit.RequestMetadata{
			RequestID: requestID,
			IPAddress: strings.TrimSpace(c.RealIP()),
			UserAgent: strings.TrimSpace(request.UserAgent()),
		}
		c.SetRequest(request.WithContext(audit.ContextWithRequestMetadata(request.Context(), metadata)))
		return next(c)
	}
}
