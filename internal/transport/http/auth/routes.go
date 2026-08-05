package auth

import "github.com/labstack/echo/v5"

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
) {
	auth := router.Group("/auth")
	auth.Use(csrfMiddleware)

	auth.POST("/register", handler.Register)
	auth.POST("/login", handler.Login)
	auth.POST("/login/mfa/totp", handler.CompleteMFATOTP)
	auth.POST("/login/mfa/recovery", handler.CompleteMFARecovery)
	auth.POST("/email/verify", handler.VerifyEmail)
	auth.POST("/email/resend", handler.ResendEmail)
	auth.POST("/password/forgot", handler.ForgotPassword)
	auth.POST("/password/reset", handler.ResetPassword)

	protected := auth.Group("")
	protected.Use(authMiddleware)

	protected.GET("/user", handler.GetUser)
	protected.POST("/logout", handler.Logout)
}
