package teamtoken

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type TenantMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
	tenantMiddleware TenantMiddleware,
) {
	tokens := router.Group("/team-tokens")
	tokens.Use(authMiddleware, csrfMiddleware)
	tokens.GET("", handler.List, tenantMiddleware(tenant.PermissionTeamTokensRead))
	tokens.POST("", handler.Create, tenantMiddleware(tenant.PermissionTeamTokensCreate))
	tokens.PATCH("/:token_id", handler.Update, tenantMiddleware(tenant.PermissionTeamTokensUpdate))
	tokens.DELETE("/:token_id", handler.Revoke, tenantMiddleware(tenant.PermissionTeamTokensRevoke))
}
