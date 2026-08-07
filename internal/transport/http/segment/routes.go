package segment

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	segments := router.Group("/segments")
	segments.POST("", handler.Create, accessMiddleware(tenant.PermissionSegmentsWrite))
	segments.GET("", handler.List, accessMiddleware(tenant.PermissionSegmentsRead))
	segments.GET("/:segment_id", handler.Get, accessMiddleware(tenant.PermissionSegmentsRead))
	segments.GET("/:segment_id/contacts", handler.ListContacts, accessMiddleware(tenant.PermissionSegmentsRead))
	segments.DELETE("/:segment_id", handler.Delete, accessMiddleware(tenant.PermissionSegmentsWrite))
}
