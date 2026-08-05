package middleware

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	"github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type PrincipalRepository interface {
	GetPrincipalByUserID(context.Context, string) (authnz.Principal, error)
}

type SessionStore interface {
	GetByTokenHash(context.Context, string) (session.Record, error)
	Touch(context.Context, string) error
}

type SessionAuthConfig struct {
	Sessions SessionStore
	Users    PrincipalRepository
}

type StepUpConfig struct {
	Assurance authnz.AssuranceLevel
	MaxAge    time.Duration
}

func RequireRecentAuthentication(config StepUpConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			principal, ok := authnz.PrincipalFromContext(c.Request().Context())
			if !ok {
				return httputil.Error(c, apperrors.NewUnauthorized("Authentication is required"))
			}
			if config.MaxAge <= 0 || !principal.RecentlyAuthenticated(config.Assurance, config.MaxAge, time.Now().UTC()) {
				return httputil.Error(c, apperrors.NewStepUpRequired("Recent stronger authentication is required"))
			}
			return next(c)
		}
	}
}

func SessionAuth(config SessionAuthConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Sessions == nil || config.Users == nil {
				return httputil.Error(c, apperrors.NewInternal("Authentication is not configured", nil))
			}
			cookie, err := c.Request().Cookie(authnz.SessionCookieName)
			if err != nil || cookie.Value == "" {
				return httputil.Error(c, apperrors.NewUnauthorized("Authentication is required"))
			}
			record, err := config.Sessions.GetByTokenHash(c.Request().Context(), authnz.HashSessionToken(cookie.Value))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
				}
				return httputil.Error(c, apperrors.NewInternal("Unable to load session", err))
			}
			if record.RevokedAt != nil || !record.ExpiresAt.After(time.Now().UTC()) {
				return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
			}
			principal, err := config.Users.GetPrincipalByUserID(c.Request().Context(), record.UserID.String())
			if err != nil {
				return httputil.Error(c, apperrors.NewInternal("Unable to resolve session user", err))
			}
			if record.CredentialVersion != principal.CredentialVersion {
				return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
			}
			if err := config.Sessions.Touch(c.Request().Context(), record.ID); err != nil {
				return httputil.Error(c, apperrors.NewInternal("Unable to update session activity", err))
			}
			principal.SessionID = record.ID
			principal.AuthenticationMethod = record.Method
			principal.AssuranceLevel = record.Assurance
			principal.AuthenticatedAt = record.AuthenticatedAt
			principal.MFACompletedAt = record.MFACompletedAt
			c.SetRequest(c.Request().WithContext(authnz.ContextWithPrincipal(c.Request().Context(), principal)))
			return next(c)
		}
	}
}

type TeamTokenStore interface {
	GetActiveByTokenHash(context.Context, string) (teamtoken.Token, error)
	Touch(context.Context, uuid.UUID) error
}

type TeamTokenConfig struct {
	Tokens   TeamTokenStore
	Required tenant.Permission
}

func TeamToken(config TeamTokenConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Tokens == nil {
				return httputil.Error(c, apperrors.NewInternal("Team token store is not configured", nil))
			}
			secret, ok := parseBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if !ok || !strings.HasPrefix(secret, teamtoken.TokenPrefix) {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid"))
			}
			token, err := config.Tokens.GetActiveByTokenHash(c.Request().Context(), authnz.HashSessionToken(secret))
			if err != nil || token.RevokedAt != nil || (token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now().UTC())) {
				return httputil.Error(c, apperrors.NewUnauthorized("Team token is invalid or expired"))
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
			access := tenant.AccessContext{Actor: tenant.Actor{Type: tenant.ActorTypeTeamToken, TokenID: tokenID}, Scope: tenant.Scope{TeamID: teamID, Status: tenant.StatusActive, Permissions: permissions}}
			if decision := tenant.Authorize(access, config.Required); !decision.Allowed {
				return httputil.Error(c, apperrors.NewForbidden(decision.Reason))
			}
			c.SetRequest(c.Request().WithContext(tenant.ContextWithAccess(c.Request().Context(), access)))
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
	return permissions, len(permissions) > 0
}
