package middlewares

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type PrincipalRepository interface {
	GetPrincipalByUserID(ctx context.Context, id string) (authnz.Principal, error)
}

type SessionStore interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (session.Record, error)
	Touch(ctx context.Context, id string) error
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
				return httputil.Error(
					c,
					apperrors.NewInternal("Authentication is not configured", nil),
				)
			}

			cookie, err := c.Request().Cookie(authnz.SessionCookieName)
			if err != nil || cookie.Value == "" {
				return httputil.Error(c, apperrors.NewUnauthorized("Authentication is required"))
			}

			session, err := config.Sessions.GetByTokenHash(
				c.Request().Context(),
				authnz.HashSessionToken(cookie.Value),
			)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
				}
				return httputil.Error(c, apperrors.NewInternal("Unable to load session", err))
			}
			if session.RevokedAt != nil || !session.ExpiresAt.After(time.Now().UTC()) {
				return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
			}

			user, err := config.Users.GetPrincipalByUserID(
				c.Request().Context(),
				session.UserID.String(),
			)
			if err != nil {
				return httputil.Error(c, apperrors.NewInternal("Unable to resolve session user", err))
			}
			if session.CredentialVersion != user.CredentialVersion {
				return httputil.Error(c, apperrors.NewUnauthorized("Session is invalid or expired"))
			}

			if err := config.Sessions.Touch(c.Request().Context(), session.ID); err != nil {
				return httputil.Error(c, apperrors.NewInternal("Unable to update session activity", err))
			}
			principal := user
			principal.SessionID = session.ID
			principal.AuthenticationMethod = session.Method
			principal.AssuranceLevel = session.Assurance
			principal.AuthenticatedAt = session.AuthenticatedAt
			principal.MFACompletedAt = session.MFACompletedAt
			ctx := authnz.ContextWithPrincipal(c.Request().Context(), principal)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
