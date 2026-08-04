package middlewares

import (
	"net"
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
		ipAddress := strings.TrimSpace(request.RemoteAddr)
		if host, _, err := net.SplitHostPort(ipAddress); err == nil {
			ipAddress = host
		}
		metadata := audit.RequestMetadata{RequestID: requestID, IPAddress: ipAddress, UserAgent: strings.TrimSpace(request.UserAgent())}
		c.SetRequest(request.WithContext(audit.ContextWithRequestMetadata(request.Context(), metadata)))
		return next(c)
	}
}
