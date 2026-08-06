package users

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	middleware ...echo.MiddlewareFunc,
) {
	group := router.Group("/users")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:user_id", handler.Detail)
}
