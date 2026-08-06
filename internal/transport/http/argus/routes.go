package argus

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, access AccessMiddleware) {
	verifications := router.Group("/verifications")
	verifications.POST("", handler.Create, access(tenant.PermissionVerifySend))
	verifications.GET("", handler.List, access(tenant.PermissionVerifyRead))
	verifications.GET("/:verification_id", handler.Get, access(tenant.PermissionVerifyRead))
	verifications.POST("/:verification_id/check", handler.Check, access(tenant.PermissionVerifyCheck))
	verifications.POST("/:verification_id/resend", handler.Resend, access(tenant.PermissionVerifySend))
	verifications.POST("/:verification_id/cancel", handler.Cancel, access(tenant.PermissionVerifySend))
}
