package senderids

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	middleware ...echo.MiddlewareFunc,
) {
	group := router.Group("/sender-ids")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:sender_id", handler.Detail)
	group.POST("/:sender_id/status", handler.UpdateStatus)
}
