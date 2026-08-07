package broadcast

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, access AccessMiddleware) {
	broadcasts := router.Group("/broadcasts")
	broadcasts.POST("", handler.Create, access(tenant.PermissionBroadcastsWrite))
	broadcasts.GET("", handler.List, access(tenant.PermissionBroadcastsRead))
	broadcasts.GET("/:broadcast", handler.Get, access(tenant.PermissionBroadcastsRead))
	broadcasts.PATCH("/:broadcast", handler.Update, access(tenant.PermissionBroadcastsWrite))
	broadcasts.DELETE("/:broadcast", handler.Delete, access(tenant.PermissionBroadcastsWrite))
	broadcasts.POST("/:broadcast/send", handler.Send, access(tenant.PermissionBroadcastsSend))
	broadcasts.POST("/:broadcast/cancel", handler.Cancel, access(tenant.PermissionBroadcastsSend))
	broadcasts.POST("/:broadcast/duplicate", handler.Duplicate, access(tenant.PermissionBroadcastsWrite))
	broadcasts.POST("/:broadcast/preview", handler.Preview, access(tenant.PermissionBroadcastsRead))
	broadcasts.GET("/:broadcast/recipients", handler.ListRecipients, access(tenant.PermissionBroadcastsRead))
}
