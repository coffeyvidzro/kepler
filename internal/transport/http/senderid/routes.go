package senderid

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	senderIDs := router.Group("/sender-ids")
	senderIDs.GET("", handler.List, accessMiddleware(tenant.PermissionSenderIDsRead))
	senderIDs.POST("", handler.Create, accessMiddleware(tenant.PermissionSenderIDsCreate))
	senderIDs.GET("/:sender_id", handler.Get, accessMiddleware(tenant.PermissionSenderIDsRead))
	senderIDs.DELETE("/:sender_id", handler.Delete, accessMiddleware(tenant.PermissionSenderIDsDelete))
}
