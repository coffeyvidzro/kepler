package messagetemplate

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	templates := router.Group("/templates")
	templates.POST("", handler.Create, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.GET("", handler.List, accessMiddleware(tenant.PermissionTemplatesRead))
	templates.GET("/:template", handler.Get, accessMiddleware(tenant.PermissionTemplatesRead))
	templates.PATCH("/:template", handler.Update, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.DELETE("/:template", handler.Delete, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.POST("/:template/publish", handler.Publish, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.POST("/:template/duplicate", handler.Duplicate, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.GET("/:template/versions", handler.ListVersions, accessMiddleware(tenant.PermissionTemplatesRead))
	templates.GET("/:template/versions/:version_id", handler.GetVersion, accessMiddleware(tenant.PermissionTemplatesRead))
	templates.POST("/:template/versions/:version_id/revert", handler.Revert, accessMiddleware(tenant.PermissionTemplatesWrite))
	templates.POST("/:template/preview", handler.Preview, accessMiddleware(tenant.PermissionTemplatesRead))
	templates.POST("/:template/test-send", handler.TestSend, accessMiddleware(tenant.PermissionTemplatesWrite))
}
