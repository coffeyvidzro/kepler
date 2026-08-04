package domain

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	domains := router.Group("/domains")
	domains.GET("", handler.List, accessMiddleware(tenant.PermissionSenderDomainsRead))
	domains.POST("", handler.Create, accessMiddleware(tenant.PermissionSenderDomainsCreate))
	domains.GET("/:domain_id", handler.Get, accessMiddleware(tenant.PermissionSenderDomainsRead))
	domains.POST("/:domain_id/verify", handler.Verify, accessMiddleware(tenant.PermissionSenderDomainsCreate))
	domains.DELETE("/:domain_id", handler.Delete, accessMiddleware(tenant.PermissionSenderDomainsDelete))
}
