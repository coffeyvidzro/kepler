package suppression

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	suppressions := router.Group("/suppressions")
	suppressions.POST("", handler.Create, accessMiddleware(tenant.PermissionSuppressionsWrite))
	suppressions.GET("", handler.List, accessMiddleware(tenant.PermissionSuppressionsRead))
	suppressions.GET("/:suppression", handler.Get, accessMiddleware(tenant.PermissionSuppressionsRead))
	suppressions.DELETE("/:suppression", handler.Delete, accessMiddleware(tenant.PermissionSuppressionsWrite))
}
