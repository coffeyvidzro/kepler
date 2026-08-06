package dashboard

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("")
	group.Use(middleware...)
	group.GET("/", handler.Index)
}
