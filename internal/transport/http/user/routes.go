package user

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
) {
	users := router.Group("/users")

	users.Use(authMiddleware)
	users.Use(csrfMiddleware)

	users.GET("/me", handler.GetMe)
	users.PATCH("/me", handler.UpdateMe)
	users.DELETE("/me", handler.DeleteMe)
	users.PATCH("/password", handler.UpdatePassword)
	users.PATCH("/email", handler.UpdateEmail)
}
