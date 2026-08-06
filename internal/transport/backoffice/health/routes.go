package health

import "github.com/labstack/echo/v5"

func RegisterRoutes(router *echo.Echo, handler *Handler) {
	router.GET("/health", handler.Live)
	router.GET("/ready", handler.Ready)
}
