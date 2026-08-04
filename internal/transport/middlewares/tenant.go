package middlewares

import (
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

const (
	defaultTenantParam  = "team_id"
	defaultTenantHeader = "X-Team-ID"
)

type TenantConfig struct {
	Memberships tenant.MembershipStore
	ParamName   string
	HeaderName  string
	Required    tenant.Permission
}

func Tenant(config TenantConfig) echo.MiddlewareFunc {
	paramName := config.ParamName
	if paramName == "" {
		paramName = defaultTenantParam
	}
	headerName := config.HeaderName
	if headerName == "" {
		headerName = defaultTenantHeader
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Memberships == nil {
				return httputil.Error(
					c,
					apperrors.NewInternal("Tenant membership store is not configured", nil),
				)
			}

			principal, ok := authnz.PrincipalFromContext(c.Request().Context())
			if !ok {
				return httputil.Error(c, apperrors.NewUnauthorized("Authentication is required"))
			}

			teamID, err := teamIDFromRequest(c, paramName, headerName)
			if err != nil {
				return httputil.Error(c, err)
			}

			membership, err := config.Memberships.GetTenantMembership(
				c.Request().Context(),
				teamID,
				principal.UserID,
			)
			if err != nil || !membership.Active() {
				return httputil.Error(
					c,
					apperrors.NewForbidden("Active team membership is required"),
				)
			}
			access := tenant.AccessContext{
				Actor: tenant.Actor{
					Type:      tenant.ActorTypeUser,
					UserID:    membership.UserID,
					SessionID: principal.SessionID,
				},
				Scope: tenant.Scope{
					TeamID: membership.TeamID,
					Role:   membership.Role,
					Status: membership.Status,
				},
			}
			if decision := tenant.Authorize(access, config.Required); !decision.Allowed {
				return httputil.Error(c, apperrors.NewForbidden(decision.Reason))
			}

			ctx := tenant.ContextWithAccess(c.Request().Context(), access)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func teamIDFromRequest(c *echo.Context, paramName string, headerName string) (uuid.UUID, error) {
	pathTeamID := strings.TrimSpace(c.Param(paramName))
	headerTeamID := strings.TrimSpace(c.Request().Header.Get(headerName))
	if pathTeamID == "" && headerTeamID == "" {
		return uuid.Nil, apperrors.NewBadRequest("Team id is required")
	}

	var parsedPathTeamID uuid.UUID
	if pathTeamID != "" {
		var err error
		parsedPathTeamID, err = uuid.Parse(pathTeamID)
		if err != nil {
			return uuid.Nil, apperrors.NewBadRequest("Team id must be a valid UUID")
		}
	}
	var parsedHeaderTeamID uuid.UUID
	if headerTeamID != "" {
		var err error
		parsedHeaderTeamID, err = uuid.Parse(headerTeamID)
		if err != nil {
			return uuid.Nil, apperrors.NewBadRequest("Team id must be a valid UUID")
		}
	}
	if pathTeamID != "" && headerTeamID != "" && parsedPathTeamID != parsedHeaderTeamID {
		return uuid.Nil, apperrors.NewBadRequest("Team id in path and header must match")
	}
	if pathTeamID != "" {
		return parsedPathTeamID, nil
	}
	return parsedHeaderTeamID, nil
}
