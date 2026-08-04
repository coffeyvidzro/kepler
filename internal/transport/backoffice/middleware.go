package backoffice

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

func RequireAdmin(adminEmails []string) echo.MiddlewareFunc {
	allowed := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			allowed[email] = struct{}{}
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if len(allowed) == 0 {
				return c.String(http.StatusForbidden, "backoffice admin allowlist is not configured")
			}

			principal, ok := authnz.PrincipalFromContext(c.Request().Context())
			if !ok {
				return c.String(http.StatusUnauthorized, "authentication is required")
			}

			if _, ok := allowed[strings.ToLower(principal.Email)]; !ok {
				return c.String(http.StatusForbidden, "backoffice access is forbidden")
			}

			return next(c)
		}
	}
}
