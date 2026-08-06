package allowancepolicies

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("/billing/allowance-policies")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:policy_id", handler.Get)
	group.POST("", handler.Create)
	group.POST("/:policy_id/close", handler.Close)
}
