package middlewares

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// NewSecure creates the application's security-header middleware.
func NewSecure(development bool) echo.MiddlewareFunc {
	config := middleware.DefaultSecureConfig

	config.XFrameOptions = "DENY"
	config.ReferrerPolicy = "strict-origin-when-cross-origin"
	config.ContentSecurityPolicy = "default-src 'self'"
	config.HSTSExcludeSubdomains = true

	if !development {
		config.HSTSMaxAge = 31536000 // 1 year
	}

	return middleware.SecureWithConfig(config)
}
