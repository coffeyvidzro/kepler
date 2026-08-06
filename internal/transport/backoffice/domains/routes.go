package domains

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	middleware ...echo.MiddlewareFunc,
) {
	domains := router.Group("/domains")
	domains.Use(middleware...)
	domains.GET("", handler.List)
	domains.GET("/:domain_id", handler.Detail)
}
