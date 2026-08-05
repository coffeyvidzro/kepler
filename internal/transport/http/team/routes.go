package team

import (
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type TenantMiddleware func(permission tenant.Permission) echo.MiddlewareFunc

func RegisterRoutes(
	router *echo.Echo,
	handler *Handler,
	authMiddleware echo.MiddlewareFunc,
	csrfMiddleware echo.MiddlewareFunc,
	tenantMiddleware TenantMiddleware,
) {
	teams := router.Group("/teams")
	teams.Use(authMiddleware, csrfMiddleware)

	teams.GET("", handler.List)
	teams.POST("", handler.Create)
	teams.GET("/invitations/:token", handler.GetInvitation)
	teams.POST("/invitations/:token/accept", handler.AcceptInvitation)
	teams.POST("/invitations/:token/decline", handler.DeclineInvitation)

	teamRoutes := teams.Group("/:team_id")
	teamRoutes.GET("", handler.Get, tenantMiddleware(tenant.PermissionTeamRead))
	teamRoutes.PATCH("", handler.Update, tenantMiddleware(tenant.PermissionTeamUpdate))
	teamRoutes.DELETE("", handler.Delete, tenantMiddleware(tenant.PermissionTeamDelete))

	teamRoutes.GET(
		"/members",
		handler.ListMembers,
		tenantMiddleware(tenant.PermissionTeamMembersRead),
	)
	teamRoutes.POST(
		"/members/invite",
		handler.InviteMember,
		tenantMiddleware(tenant.PermissionTeamMemberInvite),
	)
	teamRoutes.DELETE(
		"/members/leave",
		handler.Leave,
		tenantMiddleware(tenant.PermissionTeamMemberLeave),
	)
	teamRoutes.DELETE(
		"/members/:user_id",
		handler.RemoveMember,
		tenantMiddleware(tenant.PermissionTeamMemberRemove),
	)
	teamRoutes.PATCH(
		"/members/:user_id",
		handler.UpdateMemberRole,
		tenantMiddleware(tenant.PermissionTeamMemberRole),
	)
}
