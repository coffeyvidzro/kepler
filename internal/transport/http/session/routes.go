package session

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
) {
	sessions := router.Group("/sessions")

	sessions.Use(authMiddleware)
	sessions.Use(csrfMiddleware)

	sessions.GET("", handler.List)
	sessions.GET("/:id", handler.Get)
	sessions.DELETE("/others", handler.RevokeOthers)
	sessions.DELETE("/:id", handler.Revoke)
	sessions.DELETE("", handler.RevokeAll)
}
