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
	TokenLookup    string
	CookieName     string
}

func CSRF(config CSRFConfig) echo.MiddlewareFunc {
	tokenLookup := config.TokenLookup
	if tokenLookup == "" {
		tokenLookup = "header:" + echo.HeaderXCSRFToken
	}

	cookieName := config.CookieName
	if cookieName == "" {
		cookieName = "dugble_csrf"
	}

	return echomiddleware.CSRFWithConfig(echomiddleware.CSRFConfig{
		TrustedOrigins: config.TrustedOrigins,
		TokenLookup:    tokenLookup,
		ContextKey:     CSRFContextKey,
		CookieName:     cookieName,
		CookiePath:     "/",
		CookieSecure:   !config.Development,
		CookieHTTPOnly: false,
		CookieSameSite: http.SameSiteLaxMode,
	})
}
