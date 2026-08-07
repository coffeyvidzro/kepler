package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/dns/netdns"
	newrelicmonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/newrelic"
	sentrymonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/sentry"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	redisadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/redis"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/outbound"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms/outbound"
	auditeventmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auditevent"
	authmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	contactmodule "github.com/coffeyvidzro/dugble/server/internal/modules/contact"
	contactpropertymodule "github.com/coffeyvidzro/dugble/server/internal/modules/contactproperty"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	tenantprovision "github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant/provisioning"
	messagetemplatemodule "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
	mfamodule "github.com/coffeyvidzro/dugble/server/internal/modules/mfa"
	segmentmodule "github.com/coffeyvidzro/dugble/server/internal/modules/segment"
	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	sessionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/session"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	suppressionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/suppression"
	teammodule "github.com/coffeyvidzro/dugble/server/internal/modules/team"
	teamtokenmodule "github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	topicmodule "github.com/coffeyvidzro/dugble/server/internal/modules/topic"
	usermodule "github.com/coffeyvidzro/dugble/server/internal/modules/user"
	walletmodule "github.com/coffeyvidzro/dugble/server/internal/modules/wallet"
	webhooksmodule "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	auditeventhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auditevent"
	authhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/auth"
	broadcasthttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/broadcast"
	contacthttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/contact"
	contactpropertyhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/contactproperty"
	domainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/domain"
	emailhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/email"
	healthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/health"
	messagetemplatehttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/messagetemplate"
	mfahttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/mfa"
	segmenthttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/segment"
	senderidhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/senderid"
	sessionhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/session"
	smshttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/sms"
	suppressionhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/suppression"
	teamhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/team"
	teamtokenhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/teamtoken"
	topichttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/topic"
	userhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/user"
	wallethttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/wallet"
	webhookshttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/webhooks"
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
	if err := sentrymonitoring.Init(cfg.Sentry, cfg.AppEnv); err != nil {
		return fail(fmt.Errorf("initialize Sentry: %w", err))
	}
	cleanups.Add(func() { sentrymonitoring.Flush(5 * time.Second) })

	newRelic, err := newrelicmonitoring.New("dugble-api", cfg.AppEnv, cfg.NewRelic)
	if err != nil {
		return fail(fmt.Errorf("initialize New Relic: %w", err))
	}
	cleanups.Add(func() { newrelicmonitoring.Shutdown(newRelic, 5*time.Second) })

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

	arcjetClient, err := newArcjetClient(cfg)
	if err != nil {
		return fail(fmt.Errorf("initialize Arcjet client: %w", err))
	}
	renderer, err := systemmail.NewRenderer()
	if err != nil {
		return fail(fmt.Errorf("initialize system email renderer: %w", err))
	}
	emailClient, err := newEmailClient(cfg)
	if err != nil {
		return fail(fmt.Errorf("initialize email client: %w", err))
	}

	outboxRepository := outbox.NewRepository(db)
	systemEmailQueue := newSystemEmailQueue(cfg, outboxRepository)
	snsHandler := newProviderSNSHandler(cfg, db, outboxRepository)
	smsSender, err := newSMSSender(cfg)
	if err != nil {
		return fail(err)
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
	contactRepository := contactmodule.NewRepository(db)
	contactPropertyRepository := contactpropertymodule.NewRepository(db)
	segmentRepository := segmentmodule.NewRepository(db)
	topicRepository := topicmodule.NewRepository(db)
	suppressionRepository := suppressionmodule.NewRepository(db)
	messageTemplateRepository := messagetemplatemodule.NewRepository(db)
	broadcastRepository := broadcastmodule.NewRepository(db)
	domainRepository := domainmodule.NewRepository(db)
	emailTenantRepository := emailtenant.NewRepository(db)
	emailTenantService := emailtenant.NewService(
		emailTenantRepository,
		tenantprovision.NewQueue(outboxRepository),
	)
	senderIDRepository := senderidmodule.NewRepository(db)
	webhookRepository := webhooksmodule.NewRepository(db)
	webhookEmitter := platformwebhook.NewEmitter(webhookRepository)
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
	messageTemplateService := messagetemplatemodule.NewService(messageTemplateRepository, emailAPIService)
	broadcastService := broadcastmodule.NewService(broadcastRepository, messageTemplateService)
	webhookService := webhooksmodule.NewService(webhookRepository, webhookEmitter)
	domainService := domainmodule.NewService(domainRepository, emailClient, netdns.New(), emailTenantService)

	serverMiddleware := newServerMiddleware(serverMiddlewareDependencies{
		config:              cfg,
		sessionRepository:   sessionRepository,
		authRepository:      authRepository,
		teamRepository:      teamRepository,
		teamTokenRepository: teamTokenRepository,
	})
	routeHandlers := serverRouteHandlers{
		health:          healthhttp.NewHandler(db, redisClient),
		providerSNS:     snsHandler,
		auth:            authhttp.NewHandler(authService, cfg.IsDevelopment(), cfg.CookieDomain),
		mfa:             mfahttp.NewHandler(mfaService),
		user:            userhttp.NewHandler(usermodule.NewService(userRepository, notificationEmailService)),
		team:            teamhttp.NewHandler(teamService),
		wallet:          wallethttp.NewHandler(walletmodule.NewService(walletmodule.NewRepository(db))),
		auditEvent:      auditeventhttp.NewHandler(auditeventmodule.NewService(auditRepository)),
		contact:         contacthttp.NewHandler(contactmodule.NewService(contactRepository)),
		contactProperty: contactpropertyhttp.NewHandler(contactpropertymodule.NewService(contactPropertyRepository)),
		segment:         segmenthttp.NewHandler(segmentmodule.NewService(segmentRepository)),
		topic:           topichttp.NewHandler(topicmodule.NewService(topicRepository)),
		suppression:     suppressionhttp.NewHandler(suppressionmodule.NewService(suppressionRepository)),
		messageTemplate: messagetemplatehttp.NewHandler(messageTemplateService),
		broadcast:       broadcasthttp.NewHandler(broadcastService),
		teamToken: teamtokenhttp.NewHandler(
			teamtokenmodule.NewService(teamTokenRepository).WithNotifier(notificationEmailService),
		),
		senderID: senderidhttp.NewHandler(senderidmodule.NewService(senderIDRepository)),
		domain:   domainhttp.NewHandler(domainService),
		sms:      smshttp.NewHandler(smsService),
		email:    emailhttp.NewHandler(emailAPIService),
		webhooks: webhookshttp.NewHandler(webhookService),
		session:  sessionhttp.NewHandler(sessionmodule.NewService(sessionRepository)),
	}

	router, err := httptransport.NewRouter(
		newRouterConfig(cfg, arcjetClient, db),
		newRouteRegistrar(routeHandlers, serverMiddleware),
	)
	if err != nil {
		return fail(fmt.Errorf("create HTTP router: %w", err))
	}

	application, err := NewApplication(newrelicmonitoring.WrapHTTP(newRelic, router), ":"+cfg.HTTPPort)
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
