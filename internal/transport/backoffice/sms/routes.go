package sms

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	middleware ...echo.MiddlewareFunc,
) {
	group := router.Group("/sms")
	group.Use(middleware...)
	group.GET("", handler.List)
	group.GET("/:sms_id", handler.Detail)
}
