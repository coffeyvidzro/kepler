package backoffice

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	backofficedashboard "github.com/coffeyvidzro/dugble/server/internal/backoffice/dashboard"
	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	backofficesenderids "github.com/coffeyvidzro/dugble/server/internal/backoffice/senderids"
	backofficesms "github.com/coffeyvidzro/dugble/server/internal/backoffice/sms"
	backofficeteams "github.com/coffeyvidzro/dugble/server/internal/backoffice/teams"
	backofficeusers "github.com/coffeyvidzro/dugble/server/internal/backoffice/users"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
)

type Dependencies struct {
	DB *pgxpool.Pool
}

func NewRouter(cfg *config.Config, deps Dependencies) (*echo.Echo, error) {
	router := echo.New()

	renderer, err := NewRenderer()
	if err != nil {
		return nil, fmt.Errorf("create backoffice renderer: %w", err)
	}
	router.Renderer = renderer

	router.Use(middleware.RequestID())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.Recover())
	router.Use(middlewares.NewSecure(cfg.IsDevelopment()))

	if err := RegisterAssets(router); err != nil {
		return nil, fmt.Errorf("register backoffice assets: %w", err)
	}

	healthHandler := NewHealthHandler(deps.DB)
	router.GET("/health", healthHandler.Live)
	router.GET("/ready", healthHandler.Ready)

	sessionRepository := session.NewRepository(deps.DB)
	authRepository := auth.NewRepository(deps.DB)
	authMiddleware := middlewares.SessionAuth(middlewares.SessionAuthConfig{
		Sessions: sessionRepository,
		Users:    authRepository,
	})
	csrfMiddleware := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TrustedOrigins: cfg.CORSOrigins,
		TokenLookup:    "form:csrf,header:" + echo.HeaderXCSRFToken,
		ContextKey:     middlewares.CSRFContextKey,
		CookieName:     "dugble_backoffice_csrf",
		CookiePath:     "/",
		CookieSecure:   !cfg.IsDevelopment(),
		CookieHTTPOnly: false,
		CookieSameSite: http.SameSiteLaxMode,
	})

	teamService := backofficeteams.NewService(backofficeteams.NewRepository(deps.DB))
	handler := NewHandler(
		backofficedashboard.NewService(backofficedashboard.NewRepository(deps.DB)),
		backofficeusers.NewService(backofficeusers.NewRepository(deps.DB)),
		backofficesms.NewService(backofficesms.NewRepository(deps.DB)),
		teamService,
		backofficesenderids.NewService(backofficesenderids.NewRepository(deps.DB)),
		backofficedomains.NewService(backofficedomains.NewRepository(deps.DB)),
	)

	protected := router.Group("")
	protected.Use(authMiddleware)
	protected.Use(RequireAdmin(cfg.Backoffice.AdminEmails))
	protected.Use(csrfMiddleware)

	protected.GET("/", handler.Dashboard)
	protected.GET("/users", handler.Users)
	protected.GET("/users/:id", handler.UserDetail)
	protected.GET("/teams", handler.Teams)
	protected.GET("/teams/:id", handler.TeamDetail)
	protected.POST("/teams/:id/status", handler.UpdateTeamStatus)
	protected.GET("/sms", handler.SMSMessages)
	protected.GET("/sms/:id", handler.SMSDetail)
	protected.GET("/sender-ids", handler.SenderIDs)
	protected.GET("/sender-ids/:id", handler.SenderIDDetail)
	protected.POST("/sender-ids/:id/status", handler.UpdateSenderIDStatus)
	protected.GET("/domains", handler.Domains)
	protected.GET("/domains/:id", handler.DomainDetail)
	protected.POST("/domains/:id/status", handler.UpdateDomainStatus)

	return router, nil
}
