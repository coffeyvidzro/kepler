package contact

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	contacts := router.Group("/contacts")
	contacts.POST("", handler.Create, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.GET("", handler.List, accessMiddleware(tenant.PermissionContactsRead))
	contacts.GET("/:contact_id/topics", handler.ListTopics, accessMiddleware(tenant.PermissionContactsRead))
	contacts.PATCH("/:contact_id/topics", handler.UpdateTopics, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.GET("/:contact_id/segments", handler.ListSegments, accessMiddleware(tenant.PermissionContactsRead))
	contacts.POST("/:contact_id/segments/:segment_id", handler.AddSegment, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.DELETE("/:contact_id/segments/:segment_id", handler.RemoveSegment, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.GET("/:contact_id", handler.Get, accessMiddleware(tenant.PermissionContactsRead))
	contacts.PATCH("/:contact_id", handler.Update, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.DELETE("/:contact_id", handler.Delete, accessMiddleware(tenant.PermissionContactsWrite))
}
