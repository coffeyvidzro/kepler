package messagetemplate

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, tenantAccess echo.MiddlewareFunc) {
	group := router.Group("/templates", tenantAccess)
	group.POST("", handler.Create)
	group.GET("", handler.List)
	group.GET("/:template", handler.Get)
	group.PATCH("/:template", handler.Update)
	group.DELETE("/:template", handler.Delete)
	group.POST("/:template/publish", handler.Publish)
	group.POST("/:template/duplicate", handler.Duplicate)
	group.GET("/:template/versions", handler.ListVersions)
	group.GET("/:template/versions/:version_id", handler.GetVersion)
	group.POST("/:template/versions/:version_id/revert", handler.Revert)
	group.POST("/:template/preview", handler.Preview)
	group.POST("/:template/test-send", handler.TestSend)
}
