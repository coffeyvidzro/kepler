package server

import (
	"github.com/arcjet/arcjet-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	authmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	sessionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/session"
	teammodule "github.com/coffeyvidzro/dugble/server/internal/modules/team"
	teamtokenmodule "github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
)

type serverMiddleware struct {
	auth         echo.MiddlewareFunc
	csrf         echo.MiddlewareFunc
	tenant       func(tenant.Permission) echo.MiddlewareFunc
	tenantAccess func(tenant.Permission) echo.MiddlewareFunc
}

type serverMiddlewareDependencies struct {
	config              *config.Config
	sessionRepository   *sessionmodule.Repository
	authRepository      *authmodule.Repository
	teamRepository      *teammodule.Repository
	teamTokenRepository *teamtokenmodule.Repository
}

func newServerMiddleware(dependencies serverMiddlewareDependencies) serverMiddleware {
	authMiddleware := httpmiddleware.SessionAuth(httpmiddleware.SessionAuthConfig{
		Sessions: dependencies.sessionRepository,
		Users:    dependencies.authRepository,
	})
	csrfConfig := httpmiddleware.CSRFConfig{
		Development:    dependencies.config.IsDevelopment(),
		TrustedOrigins: dependencies.config.CORSOrigins,
	}
	csrfMiddleware := httpmiddleware.CSRF(csrfConfig)
	tenantMiddleware := func(permission tenant.Permission) echo.MiddlewareFunc {
		return httpmiddleware.Tenant(httpmiddleware.TenantConfig{
			Memberships: dependencies.teamRepository,
			Required:    permission,
		})
	}
	tenantAccess := func(permission tenant.Permission) echo.MiddlewareFunc {
		return httpmiddleware.TenantAccess(httpmiddleware.TenantAccessConfig{
			Sessions:    dependencies.sessionRepository,
			Users:       dependencies.authRepository,
			Memberships: dependencies.teamRepository,
			Tokens:      dependencies.teamTokenRepository,
			CSRF:        csrfConfig,
			Required:    permission,
		})
	}
	return serverMiddleware{
		auth:         authMiddleware,
		csrf:         csrfMiddleware,
		tenant:       tenantMiddleware,
		tenantAccess: tenantAccess,
	}
}

func newRouterConfig(cfg *config.Config, arcjetClient *arcjet.Client, db *pgxpool.Pool) httptransport.RouterConfig {
	return httptransport.RouterConfig{
		Development: cfg.IsDevelopment(),
		CORSOrigins: cfg.CORSOrigins,
		Arcjet:      arcjetClient,
		BodyLimit:   platformemail.MaxHTTPRequestBytes,
		Idempotency: httpmiddleware.IdempotencyConfig{
			Repository: idempotency.NewRepository(db),
		},
		Middleware: []echo.MiddlewareFunc{
			httpmiddleware.NewRelic(),
			sentryecho.New(sentryecho.Options{Repanic: true, WaitForDelivery: false}),
			httpmiddleware.SentryErrors(),
		},
	}
}
