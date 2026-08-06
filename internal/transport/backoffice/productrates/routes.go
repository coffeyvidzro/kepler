package productrates

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("/billing/product-rates")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:rate_id", handler.Get)
	group.POST("", handler.Create)
	group.POST("/:rate_id/close", handler.Close)
}
