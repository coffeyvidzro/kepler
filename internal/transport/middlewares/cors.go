package middlewares

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// NewCORS creates the application's CORS middleware.
func NewCORS(
	allowOrigins []string,
	development bool,
) echo.MiddlewareFunc {
	config := middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowCredentials: true,

		AllowMethods: []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},

		AllowHeaders: []string{
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			echo.HeaderContentType,
			echo.HeaderContentLength,
			echo.HeaderCacheControl,
			echo.HeaderOrigin,
			echo.HeaderXCSRFToken,
			echo.HeaderXRequestID,
			echo.HeaderXForwardedFor,
			echo.HeaderXCorrelationID,
		},

		ExposeHeaders: []string{
			echo.HeaderXCSRFToken,
			echo.HeaderXRequestID,
			echo.HeaderXCorrelationID,
		},

		MaxAge: int(12 * time.Hour / time.Second),
	}

	return middleware.CORSWithConfig(config)
}
