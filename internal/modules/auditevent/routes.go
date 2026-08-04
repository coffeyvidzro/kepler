package auditevent

import (
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	"github.com/labstack/echo/v5"
)

type TenantMiddleware func(tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, authMiddleware, csrfMiddleware echo.MiddlewareFunc, tenantMiddleware TenantMiddleware) {
	group := router.Group("/teams/:team_id/audit-events")
	group.Use(authMiddleware, csrfMiddleware)
	group.GET("", handler.List, tenantMiddleware(tenant.PermissionAuditEventsRead))
}
