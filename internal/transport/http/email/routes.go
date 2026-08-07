package email

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type TenantMiddleware func(tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, tenantAccess TenantMiddleware) {
	emails := router.Group("/emails")
	emails.GET("", handler.List, tenantAccess(tenant.PermissionEmailRead))
	emails.POST("", handler.Send, tenantAccess(tenant.PermissionEmailSend))
	emails.POST("/batch", handler.BatchSend, tenantAccess(tenant.PermissionEmailSend))
	emails.GET("/metrics", handler.Metrics, tenantAccess(tenant.PermissionEmailRead))
	emails.POST("/:message_id/cancel", handler.Cancel, tenantAccess(tenant.PermissionEmailSend))
	emails.PATCH("/:message_id", handler.Update, tenantAccess(tenant.PermissionEmailSend))
	emails.GET("/:message_id/attachments", handler.ListAttachments, tenantAccess(tenant.PermissionEmailRead))
	emails.GET("/:message_id/attachments/:attachment_id", handler.GetAttachment, tenantAccess(tenant.PermissionEmailRead))
	emails.GET("/:message_id/attachments/:attachment_id/download", handler.DownloadAttachment, tenantAccess(tenant.PermissionEmailRead))
	emails.GET("/:message_id", handler.Get, tenantAccess(tenant.PermissionEmailRead))
}
