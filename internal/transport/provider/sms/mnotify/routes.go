package mnotify

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler) {
	router.POST("/integrations/sms/mnotify", handler.Receive)
}
