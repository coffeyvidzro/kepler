package transport

import (
	"fmt"

	"github.com/arcjet/arcjet-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	dugbleserver "github.com/coffeyvidzro/dugble/server/internal/dugble/server"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/modules/auditevent"
	"github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	"github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	"github.com/coffeyvidzro/dugble/server/internal/modules/mfa"
	"github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	"github.com/coffeyvidzro/dugble/server/internal/modules/team"
	"github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	"github.com/coffeyvidzro/dugble/server/internal/modules/user"
	verifymodule "github.com/coffeyvidzro/dugble/server/internal/modules/verify"
	"github.com/coffeyvidzro/dugble/server/internal/modules/wallet"
	"github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	"github.com/coffeyvidzro/dugble/server/internal/transport/csrf"
	"github.com/coffeyvidzro/dugble/server/internal/transport/health"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
	providersns "github.com/coffeyvidzro/dugble/server/internal/transport/provider/sns"
)

type Dependencies struct {
	DB             *pgxpool.Pool
	Redis          *redis.Client
	Arcjet         *arcjet.Client
	Sender         platformemail.Sender
	DomainProvider platformemail.DomainProvider
	DNSVerifier    platformemail.DNSVerifier
	Renderer       *notifications.Renderer
	SMSSender      smsmodule.Sender
	SMSDelivery    smsmodule.DeliveryQueue
	EmailDelivery  emailmodule.DeliveryQueue
	SNSHandler     *providersns.Handler
}

func NewRouter(cfg *config.Config, deps Dependencies) (*echo.Echo, error) {
	router := echo.New()
	router.Use(middleware.RequestID())
	router.Use(middlewares.AuditRequestContext)
	router.Use(middleware.RequestLogger())
	router.Use(middleware.Recover())
	router.Use(middleware.BodyLimit(platformemail.MaxHTTPRequestBytes))
	router.Use(middlewares.NewCORS(cfg.CORSOrigins, cfg.IsDevelopment()))
	router.Use(middlewares.NewSecure(cfg.IsDevelopment()))
	router.Use(middlewares.Arcjet(deps.Arcjet))
	if deps.DB != nil {
		router.Use(middlewares.Idempotency(middlewares.IdempotencyConfig{Repository: idempotency.NewRepository(deps.DB)}))
	}
	healthHandler := health.NewHandler(deps.DB, deps.Redis)
	router.GET("/health", healthHandler.Live)
	router.GET("/ready", healthHandler.Ready)
	if deps.SNSHandler != nil {
		providersns.RegisterRoutes(router, deps.SNSHandler)
	}

	emailService := notifications.NewEmailService(deps.Sender, deps.Renderer, cfg.FrontendURL, cfg.AWS.FromEmail)
	auditRepository := audit.NewRepository(deps.DB)
	audit.SetSink(auditRepository)
	sessionRepository := session.NewRepository(deps.DB)
	authRepository := auth.NewRepository(deps.DB)
	mfaCipher, err := authnz.NewSecretCipherKeyring(cfg.EncryptionKeys)
	if err != nil {
		return nil, err
	}
	mfaService := mfa.NewService(mfa.NewRepository(deps.DB), mfaCipher, "Dugble").WithNotifier(emailService)
	authService := auth.NewService(authRepository, sessionRepository, emailService, mfaService)
	authMiddleware := middlewares.SessionAuth(middlewares.SessionAuthConfig{Sessions: sessionRepository, Users: authRepository})
	csrfConfig := middlewares.CSRFConfig{Development: cfg.IsDevelopment(), TrustedOrigins: cfg.CORSOrigins}
	csrfMiddleware := middlewares.CSRF(csrfConfig)
	csrfHandler := csrf.NewHandler()
	router.GET("/csrf", csrfHandler.Token, csrfMiddleware)
	auth.RegisterRoutes(router, auth.NewHandler(authService, cfg.IsDevelopment(), cfg.CookieDomain), authMiddleware, csrfMiddleware)
	mfa.RegisterRoutes(router, mfa.NewHandler(mfaService), authMiddleware, csrfMiddleware)

	userRepository := user.NewRepository(deps.DB)
	mfaService.WithRecipientStore(userRepository)
	user.RegisterRoutes(router, user.NewHandler(user.NewService(userRepository, emailService)), authMiddleware, csrfMiddleware)
	teamRepository := team.NewRepository(deps.DB)
	teamService := team.NewService(teamRepository, emailService).WithRecipientStore(userRepository)
	teamTokenRepository := teamtoken.NewRepository(deps.DB)
	domainRepository := domain.NewRepository(deps.DB)
	outboxRepository := outbox.NewRepository(deps.DB)
	emailTenantRepository := emailtenant.NewRepository(deps.DB)
	emailTenantService := emailtenant.NewService(emailTenantRepository, emailtenant.NewProvisionQueue(outboxRepository))
	senderIDRepository := senderid.NewRepository(deps.DB)
	webhookRepository := webhooks.NewRepository(deps.DB)
	webhookEmitter := platformwebhook.NewEmitter(webhookRepository)
	productRuntime, err := dugbleserver.New(dugbleserver.Dependencies{WebhookEmitter: webhookEmitter})
	if err != nil {
		return nil, fmt.Errorf("initialize product runtime: %w", err)
	}
	smsRepository := smsmodule.NewRepositoryWithWebhookEmitter(deps.DB, webhookEmitter)
	tenantMiddleware := func(permission tenant.Permission) echo.MiddlewareFunc {
		return middlewares.Tenant(middlewares.TenantConfig{Memberships: teamRepository, Required: permission})
	}
	tenantAccess := func(permission tenant.Permission) echo.MiddlewareFunc {
		return middlewares.TenantAccess(middlewares.TenantAccessConfig{Sessions: sessionRepository, Users: authRepository, Memberships: teamRepository, Tokens: teamTokenRepository, CSRF: csrfConfig, Required: permission})
	}
	team.RegisterRoutes(router, team.NewHandler(teamService), authMiddleware, csrfMiddleware, tenantMiddleware)
	wallet.RegisterRoutes(router, wallet.NewHandler(wallet.NewService(wallet.NewRepository(deps.DB))), tenantAccess)
	auditevent.RegisterRoutes(router, auditevent.NewHandler(auditevent.NewService(auditRepository)), authMiddleware, csrfMiddleware, tenantMiddleware)
	teamtoken.RegisterRoutes(router, teamtoken.NewHandler(teamtoken.NewService(teamTokenRepository).WithNotifier(emailService)), authMiddleware, csrfMiddleware, tenantMiddleware)
	senderid.RegisterRoutes(router, senderid.NewHandler(senderid.NewService(senderIDRepository)), tenantAccess)
	domain.RegisterRoutes(router, domain.NewHandler(domain.NewService(domainRepository, deps.DomainProvider, deps.DNSVerifier, emailTenantService)), tenantAccess)
	billingService := platformbilling.NewService(platformbilling.NewRepository(deps.DB))
	smsService := smsmodule.NewService(smsRepository, deps.SMSSender, deps.SMSDelivery, billingService)
	smsmodule.RegisterRoutes(router, smsmodule.NewHandler(smsService), tenantAccess)
	emailRepository := emailmodule.NewRepository(deps.DB)
	emailServiceAPI := emailmodule.NewService(emailRepository, deps.EmailDelivery, emailmodule.ServiceConfig{
		DefaultFromEmail: platformemail.CustomerOnboardingIdentity,
		DefaultProvider:  domain.DefaultProvider,
		DefaultRegion:    cfg.AWS.Region,
	}, billingService)
	emailmodule.RegisterRoutes(router, emailmodule.NewHandler(emailServiceAPI), tenantAccess)
	verifySecret := []byte(cfg.Verify.HMACSecret)
	verifyCodes, err := verifymodule.NewCodeManager(verifySecret, mfaCipher)
	if err != nil {
		return nil, fmt.Errorf("initialize verify code manager: %w", err)
	}
	verifyAbuse, err := verifymodule.NewRedisAbuseControls(deps.Redis, verifySecret, verifymodule.DefaultAbusePolicy())
	if err != nil {
		return nil, fmt.Errorf("initialize verify abuse controls: %w", err)
	}
	verifyService := verifymodule.NewService(
		verifymodule.NewRepository(deps.DB),
		verifyCodes,
		verifydispatch.NewQueue(outboxRepository),
		productRuntime.Events,
	).WithAbuseControls(verifyAbuse)
	verifymodule.RegisterRoutes(router, verifymodule.NewHandler(verifyService), tenantAccess)
	webhookService := webhooks.NewService(webhookRepository, webhookEmitter)
	webhooks.RegisterRoutes(router, webhooks.NewHandler(webhookService), authMiddleware, csrfMiddleware, tenantMiddleware)
	session.RegisterRoutes(router, session.NewHandler(session.NewService(sessionRepository)), authMiddleware, csrfMiddleware)
	return router, nil
}
