package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

const CSRFContextKey = "csrf"

type CSRFConfig struct {
	Development    bool
	TrustedOrigins []string
}

func CSRF(config CSRFConfig) echo.MiddlewareFunc {
	return echomiddleware.CSRFWithConfig(echomiddleware.CSRFConfig{
		TrustedOrigins: config.TrustedOrigins,
		TokenLookup:    "header:" + echo.HeaderXCSRFToken,
		ContextKey:     CSRFContextKey,
		CookieName:     "dugble_csrf",
		CookiePath:     "/",
		CookieSecure:   !config.Development,
		CookieHTTPOnly: false,
		CookieSameSite: http.SameSiteLaxMode,
	})
}
