package contactproperty

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	properties := router.Group("/contact-properties")
	properties.POST("", handler.Create, accessMiddleware(tenant.PermissionContactPropertiesWrite))
	properties.GET("", handler.List, accessMiddleware(tenant.PermissionContactPropertiesRead))
	properties.GET("/:property_id", handler.Get, accessMiddleware(tenant.PermissionContactPropertiesRead))
	properties.PATCH("/:property_id", handler.Update, accessMiddleware(tenant.PermissionContactPropertiesWrite))
	properties.DELETE("/:property_id", handler.Delete, accessMiddleware(tenant.PermissionContactPropertiesWrite))
}
