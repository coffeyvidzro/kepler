package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/labstack/echo/v5"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/dns/netdns"
	leamoutsms "github.com/coffeyvidzro/dugble/server/internal/adapters/leamout/sms"
	mnotifyadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	mnotifysms "github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify/sms"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	moolresms "github.com/coffeyvidzro/dugble/server/internal/adapters/moolre/sms"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	redisadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/redis"
	runnagesms "github.com/coffeyvidzro/dugble/server/internal/adapters/runnage/sms"
	arcjetadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/security/arcjet"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	argusdispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/argus/dispatch"
	"github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/outbound"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms/outbound"
	argusmodule "github.com/coffeyvidzro/dugble/server/internal/modules/argus"
	auditeventmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auditevent"
	authmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	tenantprovision "github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant/provisioning"
	mfamodule "github.com/coffeyvidzro/dugble/server/internal/modules/mfa"
	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	sessionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/session"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	teammodule "github.com/coffeyvidzro/dugble/server/internal/modules/team"
	teamtokenmodule "github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	usermodule "github.com/coffeyvidzro/dugble/server/internal/modules/user"
	walletmodule "github.com/coffeyvidzro/dugble/server/internal/modules/wallet"
	webhooksmodule "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	"github.com/coffeyvidzro/dugble/server/internal/platform/monitoring"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	argushttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/argus"
	auditeventhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auditevent"
	authhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auth"
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
)

// Wire builds the server and returns a cleanup function for all initialized resources.
func Wire(ctx context.Context) (*Application, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("server wiring context is required")
	}

	cleanups := &cleanupStack{}
	fail := func(err error) (*Application, func(), error) {
		cleanups.Run()
		return nil, nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(fmt.Errorf("load configuration: %w", err))
	}
	if err := monitoring.InitSentry(cfg.Sentry, cfg.AppEnv); err != nil {
		return fail(fmt.Errorf("initialize Sentry: %w", err))
	}
	cleanups.Add(func() { monitoring.FlushSentry(5 * time.Second) })

	newRelic, err := monitoring.NewRelic("dugble-api", cfg.AppEnv, cfg.NewRelic)
	if err != nil {
		return fail(fmt.Errorf("initialize New Relic: %w", err))
	}
	cleanups.Add(func() { monitoring.Shutdown(newRelic, 5*time.Second) })

	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	db, err := postgres.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fail(fmt.Errorf("initialize PostgreSQL: %w", err))
	}
	cleanups.Add(db.Close)

	redisClient, err := redisadapter.New(startupCtx, cfg.RedisURL)
	if err != nil {
		return fail(fmt.Errorf("initialize Redis: %w", err))
	}
	cleanups.Add(func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			slog.Warn("close Redis client", "error", closeErr)
		}
	})

	arcjetClient, err := arcjetadapter.New(cfg.ArcjetKey)
	if err != nil {
		return fail(fmt.Errorf("initialize Arcjet: %w", err))
	}
	renderer, err := systemmail.NewRenderer()
	if err != nil {
		return fail(fmt.Errorf("initialize email renderer: %w", err))
	}
	emailClient, err := awsses.NewClient(
		cfg.AWS.Region,
		cfg.AWS.FromEmail,
		cfg.AWS.AccessKey,
		cfg.AWS.SecretKey,
		cfg.AWS.SESTransactionalConfigurationSet,
	)
	if err != nil {
		return fail(fmt.Errorf("initialize SES email client: %w", err))
	}

	outboxRepository := outbox.NewRepository(db)
	systemEmailQueue := systememail.NewQueue(outboxRepository, platformemail.Message{
		Provider:         awsses.ProviderSES,
		Region:           cfg.AWS.Region,
		Stream:           "transactional",
		ConfigurationSet: cfg.AWS.SESTransactionalConfigurationSet,
		SESTenantName:    cfg.AWS.SESTenantName,
	})

	var snsHandler *providersns.Handler
	if len(cfg.AWS.SNSTopicARNs) > 0 {
		certificateLoader := awssns.NewHTTPCertificateLoader(nil)
		verifier := awssns.NewVerifier(cfg.AWS.SNSTopicARNs, certificateLoader)
		confirmer := awssns.NewConfirmer(awssns.NewHTTPConfirmSubscriptionClient(nil))
		ingestor := feedback.NewRepository(db, outboxRepository)
		snsHandler = providersns.NewHandler(verifier, confirmer, ingestor)
	}

	smsRouter, err := platformsms.NewRoutingService(
		platformsms.DefaultRoutingConfig(),
		mnotifysms.NewProvider(mnotifyadapter.NewClient(cfg.MNotify.BaseURL, cfg.MNotify.APIKey)),
		moolresms.NewProvider(moolre.NewClient(cfg.Moolre.BaseURL, cfg.Moolre.VASKey)),
		leamoutsms.NewProvider(),
		runnagesms.NewProvider(),
	)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS router: %w", err))
	}
	smsSender, err := platformsms.NewService(smsRouter)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS sender: %w", err))
	}

	notificationEmailService := systemmail.NewEmailService(
		systemEmailQueue,
		renderer,
		cfg.FrontendURL,
		cfg.AWS.FromEmail,
	)
	auditRepository := audit.NewRepository(db)
	audit.SetSink(auditRepository)
	sessionRepository := sessionmodule.NewRepository(db)
	authRepository := authmodule.NewRepository(db)
	mfaCipher, err := authnz.NewSecretCipherKeyring(cfg.EncryptionKeys)
	if err != nil {
		return fail(fmt.Errorf("initialize MFA cipher: %w", err))
	}
	mfaService := mfamodule.NewService(
		mfamodule.NewRepository(db),
		mfaCipher,
		"Dugble",
	).WithNotifier(notificationEmailService)
	authService := authmodule.NewService(
		authRepository,
		sessionRepository,
		notificationEmailService,
		mfaService,
	)
	userRepository := usermodule.NewRepository(db)
	mfaService.WithRecipientStore(userRepository)
	teamRepository := teammodule.NewRepository(db)
	teamService := teammodule.NewService(
		teamRepository,
		notificationEmailService,
	).WithRecipientStore(userRepository)
	teamTokenRepository := teamtokenmodule.NewRepository(db)
	domainRepository := domainmodule.NewRepository(db)
	emailTenantRepository := emailtenant.NewRepository(db)
	emailTenantService := emailtenant.NewService(
		emailTenantRepository,
		tenantprovision.NewQueue(outboxRepository),
	)
	senderIDRepository := senderidmodule.NewRepository(db)
	webhookRepository := webhooksmodule.NewRepository(db)
	webhookEmitter := platformwebhook.NewEmitter(webhookRepository)
	productRuntime, err := New(Dependencies{WebhookEmitter: webhookEmitter})
	if err != nil {
		return fail(fmt.Errorf("initialize product runtime: %w", err))
	}

	smsRepository := smsmodule.NewRepositoryWithWebhookEmitter(db, webhookEmitter)
	billingService := platformbilling.NewService(platformbilling.NewRepository(db))
	smsService := smsmodule.NewService(
		smsRepository,
		smsSender,
		smsdelivery.NewQueue(outboxRepository),
		billingService,
	)
	emailRepository := emailmodule.NewRepository(db)
	emailAPIService := emailmodule.NewService(
		emailRepository,
		emaildelivery.NewQueue(outboxRepository),
		emailmodule.ServiceConfig{
			DefaultFromEmail: cfg.AWS.FromEmail,
			DefaultProvider:  domainmodule.DefaultProvider,
			DefaultRegion:    cfg.AWS.Region,
		},
		billingService,
	)

	argusSecret := []byte(cfg.Argus.HMACSecret)
	argusCodes, err := argusmodule.NewCodeManager(argusSecret, mfaCipher)
	if err != nil {
		return fail(fmt.Errorf("initialize verify code manager: %w", err))
	}
	argusService := argusmodule.NewService(
		argusmodule.NewRepository(db),
		argusCodes,
		argusdispatch.NewQueue(outboxRepository),
		productRuntime.Events,
	)
	webhookService := webhooksmodule.NewService(webhookRepository, webhookEmitter)

	authMiddleware := httpmiddleware.SessionAuth(httpmiddleware.SessionAuthConfig{
		Sessions: sessionRepository,
		Users:    authRepository,
	})
	csrfConfig := httpmiddleware.CSRFConfig{
		Development:    cfg.IsDevelopment(),
		TrustedOrigins: cfg.CORSOrigins,
	}
	csrfMiddleware := httpmiddleware.CSRF(csrfConfig)
	tenantMiddleware := func(permission tenant.Permission) echo.MiddlewareFunc {
		return httpmiddleware.Tenant(httpmiddleware.TenantConfig{
			Memberships: teamRepository,
			Required:    permission,
		})
	}
	tenantAccess := func(permission tenant.Permission) echo.MiddlewareFunc {
		return httpmiddleware.TenantAccess(httpmiddleware.TenantAccessConfig{
			Sessions:    sessionRepository,
			Users:       authRepository,
			Memberships: teamRepository,
			Tokens:      teamTokenRepository,
			CSRF:        csrfConfig,
			Required:    permission,
		})
	}

	registrar := func(router *echo.Echo) error {
		healthhttp.RegisterRoutes(router, healthhttp.NewHandler(db, redisClient))
		if snsHandler != nil {
			providersns.RegisterRoutes(router, snsHandler)
		}
		router.GET("/csrf", func(c *echo.Context) error {
			token, ok := c.Get(httpmiddleware.CSRFContextKey).(string)
			if !ok || token == "" {
				return httptransport.Error(
					c,
					apperrors.NewInternal("CSRF token is not available", nil),
				)
			}
			return httptransport.OK(c, map[string]string{"csrf_token": token})
		}, csrfMiddleware)

		authhttp.RegisterRoutes(
			router,
			authhttp.NewHandler(authService, cfg.IsDevelopment(), cfg.CookieDomain),
			authMiddleware,
			csrfMiddleware,
		)
		mfahttp.RegisterRoutes(
			router,
			mfahttp.NewHandler(mfaService),
			authMiddleware,
			csrfMiddleware,
		)
		userhttp.RegisterRoutes(
			router,
			userhttp.NewHandler(usermodule.NewService(userRepository, notificationEmailService)),
			authMiddleware,
			csrfMiddleware,
		)
		teamhttp.RegisterRoutes(
			router,
			teamhttp.NewHandler(teamService),
			authMiddleware,
			csrfMiddleware,
			tenantMiddleware,
		)
		wallethttp.RegisterRoutes(
			router,
			wallethttp.NewHandler(walletmodule.NewService(walletmodule.NewRepository(db))),
			tenantAccess,
		)
		auditeventhttp.RegisterRoutes(
			router,
			auditeventhttp.NewHandler(auditeventmodule.NewService(auditRepository)),
			authMiddleware,
			csrfMiddleware,
			tenantMiddleware,
		)
		teamtokenhttp.RegisterRoutes(
			router,
			teamtokenhttp.NewHandler(
				teamtokenmodule.NewService(teamTokenRepository).WithNotifier(notificationEmailService),
			),
			authMiddleware,
			csrfMiddleware,
			tenantMiddleware,
		)
		senderidhttp.RegisterRoutes(
			router,
			senderidhttp.NewHandler(senderidmodule.NewService(senderIDRepository)),
			tenantAccess,
		)
		domainhttp.RegisterRoutes(
			router,
			domainhttp.NewHandler(
				domainmodule.NewService(
					domainRepository,
					emailClient,
					netdns.New(),
					emailTenantService,
				),
			),
			tenantAccess,
		)
		smshttp.RegisterRoutes(router, smshttp.NewHandler(smsService), tenantAccess)
		emailhttp.RegisterRoutes(router, emailhttp.NewHandler(emailAPIService), tenantAccess)
		argushttp.RegisterRoutes(router, argushttp.NewHandler(argusService), tenantAccess)
		webhookshttp.RegisterRoutes(
			router,
			webhookshttp.NewHandler(webhookService),
			authMiddleware,
			csrfMiddleware,
			tenantMiddleware,
		)
		sessionhttp.RegisterRoutes(
			router,
			sessionhttp.NewHandler(sessionmodule.NewService(sessionRepository)),
			authMiddleware,
			csrfMiddleware,
		)
		return nil
	}

	router, err := httptransport.NewRouter(httptransport.RouterConfig{
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
	}, registrar)
	if err != nil {
		return fail(fmt.Errorf("create HTTP router: %w", err))
	}

	application, err := NewApplication(monitoring.WrapHTTP(newRelic, router), ":"+cfg.HTTPPort)
	if err != nil {
		return fail(fmt.Errorf("create HTTP application: %w", err))
	}
	return application, cleanups.Run, nil
}

type cleanupStack struct {
	functions []func()
}

func (stack *cleanupStack) Add(cleanup func()) {
	if stack == nil || cleanup == nil {
		return
	}
	stack.functions = append(stack.functions, cleanup)
}

func (stack *cleanupStack) Run() {
	if stack == nil {
		return
	}
	for index := len(stack.functions) - 1; index >= 0; index-- {
		stack.functions[index]()
	}
	stack.functions = nil
}
