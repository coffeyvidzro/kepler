package backoffice

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

func newBackofficeAccessMiddleware(token string) echo.MiddlewareFunc {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			supplied := bearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if supplied == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 {
				return httputil.Error(c, apperrors.New(apperrors.CodeUnauthorized, "Backoffice access token is required", http.StatusUnauthorized, nil))
			}
			return next(c)
		}
	}
}

func bearerToken(header string) string {
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}
