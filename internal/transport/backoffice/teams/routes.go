package teams

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	middleware ...echo.MiddlewareFunc,
) {
	group := router.Group("/teams")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:team_id", handler.Detail)
	group.POST("/:team_id/status", handler.UpdateStatus)
}
