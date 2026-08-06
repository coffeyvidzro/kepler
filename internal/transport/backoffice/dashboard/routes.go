package dashboard

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler, middleware ...echo.MiddlewareFunc) {
	group := router.Group("/dashboard")
	group.Use(middleware...)
	group.GET("/stats", handler.Stats)
}
