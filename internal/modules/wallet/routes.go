package wallet

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type AccessMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(router *echo.Echo, handler *Handler, accessMiddleware AccessMiddleware) {
	wallet := router.Group("/wallet")
	wallet.GET("", handler.Get, accessMiddleware(tenant.PermissionWalletRead))
	wallet.GET("/ledger", handler.ListLedger, accessMiddleware(tenant.PermissionWalletLedgerRead))
}
