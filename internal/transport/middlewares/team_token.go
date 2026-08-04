package middlewares

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type TeamTokenStore interface {
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (teamtoken.Token, error)
	Touch(ctx context.Context, id uuid.UUID) error
}

type TeamTokenConfig struct {
	Tokens   TeamTokenStore
	Required tenant.Permission
}

func TeamToken(config TeamTokenConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Tokens == nil {
				return httputil.Error(
					c,
					apperrors.NewInternal("Team token store is not configured", nil),
				)
			}
			secret, ok := parseBearerToken(c.Request().Header.Get("Authorization"))
			if !ok {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is required"))
			}
			if !strings.HasPrefix(secret, teamtoken.TokenPrefix) {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid"))
			}
			token, err := config.Tokens.GetActiveByTokenHash(
				c.Request().Context(),
				authnz.HashSessionToken(secret),
			)
			if err != nil || token.RevokedAt != nil ||
				(token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now().UTC())) {
				return httputil.Error(
					c,
					apperrors.NewUnauthorized("Team token is invalid or expired"),
				)
			}
			permissions, ok := tokenPermissions(token.Permissions)
			if !ok {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid"))
			}
			tokenID, err := uuid.Parse(token.ID)
			if err != nil {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid"))
			}
			teamID, err := uuid.Parse(token.TeamID)
			if err != nil {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid"))
			}
			_ = config.Tokens.Touch(c.Request().Context(), tokenID)
			access := tenant.AccessContext{
				Actor: tenant.Actor{Type: tenant.ActorTypeTeamToken, TokenID: tokenID},
				Scope: tenant.Scope{TeamID: teamID, Status: tenant.StatusActive, Permissions: permissions},
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

func parseBearerToken(value string) (string, bool) {
	prefix, token, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func tokenPermissions(values []string) ([]tenant.Permission, bool) {
	permissions := make([]tenant.Permission, 0, len(values))
	for _, value := range values {
		permission := tenant.Permission(strings.TrimSpace(value))
		if permission == "" || !teamtoken.IsAllowedPermission(permission) {
			return nil, false
		}
		permissions = append(permissions, permission)
	}
	if len(permissions) == 0 {
		return nil, false
	}
	return permissions, true
}
