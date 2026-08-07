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
	contacts.GET("/:contact_id", handler.Get, accessMiddleware(tenant.PermissionContactsRead))
	contacts.PATCH("/:contact_id", handler.Update, accessMiddleware(tenant.PermissionContactsWrite))
	contacts.DELETE("/:contact_id", handler.Delete, accessMiddleware(tenant.PermissionContactsWrite))
}
