package topic

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	topics := router.Group("/topics")
	topics.POST("", handler.Create, accessMiddleware(tenant.PermissionTopicsWrite))
	topics.GET("", handler.List, accessMiddleware(tenant.PermissionTopicsRead))
	topics.GET("/:topic_id", handler.Get, accessMiddleware(tenant.PermissionTopicsRead))
	topics.PATCH("/:topic_id", handler.Update, accessMiddleware(tenant.PermissionTopicsWrite))
	topics.DELETE("/:topic_id", handler.Delete, accessMiddleware(tenant.PermissionTopicsWrite))
}
