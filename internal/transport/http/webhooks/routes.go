package webhooks

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type TenantMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
	tenantMiddleware TenantMiddleware,
) {
	endpoints := router.Group("/webhook-endpoints")
	endpoints.Use(authMiddleware, csrfMiddleware)
	endpoints.POST("", handler.CreateEndpoint, tenantMiddleware(tenant.PermissionWebhooksWrite))
	endpoints.GET("", handler.ListEndpoints, tenantMiddleware(tenant.PermissionWebhooksRead))
	endpoints.GET("/:endpoint_id", handler.GetEndpoint, tenantMiddleware(tenant.PermissionWebhooksRead))
	endpoints.PATCH("/:endpoint_id", handler.UpdateEndpoint, tenantMiddleware(tenant.PermissionWebhooksWrite))
	endpoints.DELETE("/:endpoint_id", handler.DeleteEndpoint, tenantMiddleware(tenant.PermissionWebhooksWrite))
	endpoints.POST("/:endpoint_id/test", handler.TestEndpoint, tenantMiddleware(tenant.PermissionWebhooksWrite))
	endpoints.POST("/:endpoint_id/rotate-secret", handler.RotateSecret, tenantMiddleware(tenant.PermissionWebhooksWrite))

	events := router.Group("/webhook-events")
	events.Use(authMiddleware, csrfMiddleware)
	events.GET("", handler.ListEvents, tenantMiddleware(tenant.PermissionWebhooksRead))
	events.GET("/:event_id", handler.GetEvent, tenantMiddleware(tenant.PermissionWebhooksRead))

	deliveries := router.Group("/webhook-deliveries")
	deliveries.Use(authMiddleware, csrfMiddleware)
	deliveries.GET("/:delivery_id", handler.GetDelivery, tenantMiddleware(tenant.PermissionWebhooksRead))
	deliveries.POST("/:delivery_id/retry", handler.RetryDelivery, tenantMiddleware(tenant.PermissionWebhooksWrite))
}
