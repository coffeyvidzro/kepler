package mfa

import (
	"time"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
)

func RegisterRoutes(router *echo.Echo, handler *Handler, authMiddleware, csrfMiddleware echo.MiddlewareFunc) {
	group := router.Group("/auth/mfa")
	group.Use(authMiddleware)
	group.Use(csrfMiddleware)

	recentPassword := middlewares.RequireRecentAuthentication(middlewares.StepUpConfig{Assurance: authnz.AssuranceLevelOne, MaxAge: 15 * time.Minute})
	recentMFA := middlewares.RequireRecentAuthentication(middlewares.StepUpConfig{Assurance: authnz.AssuranceLevelTwo, MaxAge: 15 * time.Minute})
	group.GET("", handler.Status)
	group.POST("/totp/enroll", handler.Enroll, recentPassword)
	group.POST("/totp/confirm", handler.Confirm, recentPassword)
	group.POST("/verify", handler.Verify)
	group.POST("/recovery", handler.Recover)
	group.DELETE("", handler.Disable, recentMFA)
}
