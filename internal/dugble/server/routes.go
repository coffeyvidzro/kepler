package server

import (
	"github.com/labstack/echo/v5"

	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	auditeventhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auditevent"
	authhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auth"
	contacthttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/contact"
	contactpropertyhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/contactproperty"
	domainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/domain"
	emailhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/email"
	healthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/health"
	mfahttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/mfa"
	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
	senderidhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/senderid"
	sessionhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/session"
	smshttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/sms"
	teamhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/team"
	teamtokenhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/teamtoken"
	userhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/user"
	wallethttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/wallet"
	webhookshttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/webhooks"
	providersns "github.com/coffeyvidzro/dugble/server/internal/transport/provider/aws/sns"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type serverRouteHandlers struct {
	health           *healthhttp.Handler
	providerSNS      *providersns.Handler
	auth             *authhttp.Handler
	mfa              *mfahttp.Handler
	user             *userhttp.Handler
	team             *teamhttp.Handler
	wallet           *wallethttp.Handler
	auditEvent       *auditeventhttp.Handler
	teamToken        *teamtokenhttp.Handler
	contact          *contacthttp.Handler
	contactProperty  *contactpropertyhttp.Handler
	senderID         *senderidhttp.Handler
	domain           *domainhttp.Handler
	sms              *smshttp.Handler
	email            *emailhttp.Handler
	webhooks         *webhookshttp.Handler
	session          *sessionhttp.Handler
}

func newRouteRegistrar(handlers serverRouteHandlers, middleware serverMiddleware) httptransport.Registrar {
	return func(router *echo.Echo) error {
		healthhttp.RegisterRoutes(router, handlers.health)
		if handlers.providerSNS != nil {
			providersns.RegisterRoutes(router, handlers.providerSNS)
		}
		registerCSRFRoute(router, middleware.csrf)

		authhttp.RegisterRoutes(router, handlers.auth, middleware.auth, middleware.csrf)
		mfahttp.RegisterRoutes(router, handlers.mfa, middleware.auth, middleware.csrf)
		userhttp.RegisterRoutes(router, handlers.user, middleware.auth, middleware.csrf)
		teamhttp.RegisterRoutes(router, handlers.team, middleware.auth, middleware.csrf, middleware.tenant)
		wallethttp.RegisterRoutes(router, handlers.wallet, middleware.tenantAccess)
		auditeventhttp.RegisterRoutes(router, handlers.auditEvent, middleware.auth, middleware.csrf, middleware.tenant)
		teamtokenhttp.RegisterRoutes(router, handlers.teamToken, middleware.auth, middleware.csrf, middleware.tenant)
		contacthttp.RegisterRoutes(router, handlers.contact, middleware.tenantAccess)
		contactpropertyhttp.RegisterRoutes(router, handlers.contactProperty, middleware.tenantAccess)
		senderidhttp.RegisterRoutes(router, handlers.senderID, middleware.tenantAccess)
		domainhttp.RegisterRoutes(router, handlers.domain, middleware.tenantAccess)
		smshttp.RegisterRoutes(router, handlers.sms, middleware.tenantAccess)
		emailhttp.RegisterRoutes(router, handlers.email, middleware.tenantAccess)
		webhookshttp.RegisterRoutes(router, handlers.webhooks, middleware.auth, middleware.csrf, middleware.tenant)
		sessionhttp.RegisterRoutes(router, handlers.session, middleware.auth, middleware.csrf)
		return nil
	}
}

func registerCSRFRoute(router *echo.Echo, csrfMiddleware echo.MiddlewareFunc) {
	router.GET("/csrf", func(c *echo.Context) error {
		token, ok := c.Get(httpmiddleware.CSRFContextKey).(string)
		if !ok || token == "" {
			return httputil.Error(
				c,
				apperrors.NewInternal("CSRF token is not available", nil),
			)
		}
		return httputil.OK(c, map[string]string{"csrf_token": token})
	}, csrfMiddleware)
}
