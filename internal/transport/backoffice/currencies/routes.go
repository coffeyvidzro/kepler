package currencies

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("/billing/currencies")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:currency_code", handler.Get)
	group.POST("", handler.Create)
	group.PATCH("/:currency_code", handler.Update)
}
