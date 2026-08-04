package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sentryecho "github.com/getsentry/sentry-go/echo"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/dns/netdns"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	redisadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/redis"
	arcjetadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/security/arcjet"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/send"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms"
	smsintegration "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/arkesel"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/celcom"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/mnotify"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/routing"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/monitoring"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/transport"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
	providersns "github.com/coffeyvidzro/dugble/server/internal/transport/provider/sns"
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

	smsRouter, err := routing.NewService(
		routing.DefaultConfig(),
		arkesel.NewProvider(arkesel.NewClient(cfg.Arkesel)),
		celcom.NewProvider(celcom.NewClient(cfg.Celcom)),
		mnotify.NewProvider(mnotify.NewClient(cfg.MNotify)),
	)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS router: %w", err))
	}
	smsSender, err := smsintegration.NewService(smsRouter)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS sender: %w", err))
	}

	router, err := transport.NewRouter(cfg, transport.Dependencies{
		DB:             db,
		Redis:          redisClient,
		Arcjet:         arcjetClient,
		Sender:         systemEmailQueue,
		DomainProvider: emailClient,
		DNSVerifier:    netdns.New(),
		Renderer:       renderer,
		SMSSender:      smsSender,
		SMSDelivery:    smsdelivery.NewQueue(outboxRepository),
		EmailDelivery:  emaildelivery.NewQueue(outboxRepository),
		SNSHandler:     snsHandler,
	})
	if err != nil {
		return fail(fmt.Errorf("create HTTP router: %w", err))
	}
	router.Use(middlewares.NewRelic())
	router.Use(sentryecho.New(sentryecho.Options{Repanic: true, WaitForDelivery: false}))
	router.Use(middlewares.SentryErrors())

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
