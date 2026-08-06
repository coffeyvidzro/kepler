package billingmarkets

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("/billing/markets")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:market_code", handler.Get)
	group.POST("", handler.Create)
	group.PATCH("/:market_code", handler.Update)
}
