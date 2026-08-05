package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

func CORS(allowOrigins []string) echo.MiddlewareFunc {
	return echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins: allowOrigins, AllowCredentials: true,
		AllowMethods:  []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{echo.HeaderAccept, echo.HeaderAuthorization, echo.HeaderContentType, echo.HeaderContentLength, echo.HeaderCacheControl, echo.HeaderOrigin, echo.HeaderXCSRFToken, echo.HeaderXRequestID, echo.HeaderXForwardedFor, echo.HeaderXCorrelationID},
		ExposeHeaders: []string{echo.HeaderXCSRFToken, echo.HeaderXRequestID, echo.HeaderXCorrelationID},
		MaxAge:        int(12 * time.Hour / time.Second),
	})
}
