package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/dns/netdns"
	natsadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/nats"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/sms/arkesel"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/sms/celcom"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/sms/mnotify"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	domainreconciliation "github.com/coffeyvidzro/dugble/server/internal/delivery/domain"
	emailfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/outbound"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	tenantprovision "github.com/coffeyvidzro/dugble/server/internal/delivery/email/tenant"
	smsfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/sms/feedback"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms/outbound"
	verifycleanup "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/cleanup"
	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	verifyexpiry "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/expiry"
	webhookdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/webhook"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	webhookmodule "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

// Wire builds the worker and returns a cleanup function for initialized resources.
func Wire(ctx context.Context) (*Worker, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("worker wiring context is required")
	}

	cleanups := &cleanupStack{}
	fail := func(err error) (*Worker, func(), error) {
		cleanups.Run()
		return nil, nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(fmt.Errorf("load configuration: %w", err))
	}

	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	db, err := postgres.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fail(fmt.Errorf("initialize PostgreSQL: %w", err))
	}
	cleanups.Add(db.Close)

	messagingClient, err := natsadapter.New(startupCtx, cfg.NATSURL, "dugble-worker")
	if err != nil {
		return fail(fmt.Errorf("initialize JetStream: %w", err))
	}
	cleanups.Add(func() {
		if closeErr := messagingClient.Close(); closeErr != nil {
			slog.Warn("close JetStream client", "error", closeErr)
		}
	})
	if err := messagingClient.Provision(startupCtx, natsadapter.DefaultStreamLimits()); err != nil {
		return fail(fmt.Errorf("provision JetStream topology: %w", err))
	}

	outboxRepository := outbox.NewRepository(db)
	processedEvents := outboxRepository
	webhookModuleRepository := webhookmodule.NewRepository(db)
	webhookEmitter := platformwebhook.NewEmitter(webhookModuleRepository)
	events := platformevent.NewEmitter(platformwebhook.NewEventSink(webhookEmitter))
	lifecycleEmitter := webhookEmitter
	billingService := platformbilling.NewService(platformbilling.NewRepository(db))

	emailSender, err := awsses.NewSESSender(
		startupCtx,
		cfg.AWS.Region,
		cfg.AWS.FromEmail,
		cfg.AWS.AccessKey,
		cfg.AWS.SecretKey,
		cfg.AWS.SESConfigurationSet,
	)
	if err != nil {
		return fail(fmt.Errorf("initialize SES email sender: %w", err))
	}

	emailConsumer := emaildelivery.NewConsumer(
		messagingClient,
		processedEvents,
		emaildelivery.NewProcessor(emaildelivery.NewRepository(db), emailSender),
		emaildelivery.ConsumerConfig{
			Concurrency:    5,
			AckWait:        2 * time.Minute,
			HandlerTimeout: 45 * time.Second,
			MaxDeliver:     6,
			RetryPolicy:    emaildelivery.DefaultRetryPolicy(),
		},
	)
	systemEmailConsumer := systememail.NewConsumer(
		messagingClient,
		processedEvents,
		emailSender,
		systememail.ConsumerConfig{
			Concurrency:    3,
			AckWait:        time.Minute,
			HandlerTimeout: 30 * time.Second,
			MaxDeliver:     6,
		},
	)
	emailTenantRepository := emailtenant.NewRepository(db)
	emailTenantConsumer := tenantprovision.NewConsumer(
		messagingClient,
		processedEvents,
		tenantprovision.NewProcessor(emailTenantRepository, emailSender),
		tenantprovision.Config{
			Concurrency:    3,
			AckWait:        2 * time.Minute,
			HandlerTimeout: 60 * time.Second,
			RetryBackOff:   tenantprovision.DefaultRetryBackOff(),
		},
	)

	feedbackMetrics := emailfeedback.DefaultMetrics
	emailFeedbackRepository := emailfeedback.NewRepositoryWithWebhookEmitter(db, lifecycleEmitter)
	emailFeedbackConsumer := emailfeedback.NewConsumer(
		messagingClient,
		processedEvents,
		emailfeedback.NewHandlerWithMetrics(emailFeedbackRepository, feedbackMetrics),
		emailfeedback.ConsumerConfig{
			Concurrency:    5,
			AckWait:        time.Minute,
			HandlerTimeout: 30 * time.Second,
			MaxDeliver:     6,
			RetryPolicy:    emailfeedback.DefaultRetryPolicy(),
		},
	)
	emailFeedbackReconciler := emailfeedback.NewObservedReconciler(
		emailFeedbackRepository,
		emailfeedback.ReconcilerConfig{
			PollInterval:  5 * time.Second,
			BatchSize:     25,
			Concurrency:   5,
			LeaseDuration: 2 * time.Minute,
			HandleTimeout: 30 * time.Second,
		},
		feedbackMetrics,
	)
	emailFeedbackMetricsCollector := emailfeedback.NewMetricsCollector(db, feedbackMetrics, 15*time.Second)

	domainRepository := domainmodule.NewRepository(db)
	domainService := domainmodule.NewService(domainRepository, emailSender, netdns.New())
	domainWorkerID := "sender-domain-reconciliation-" + uuid.NewString()
	domainConsumer := domainreconciliation.NewConsumer(
		domainRepository,
		domainService,
		domainreconciliation.Config{
			PollInterval:           30 * time.Second,
			BatchSize:              25,
			Concurrency:            5,
			LockTimeout:            2 * time.Minute,
			CheckTimeout:           20 * time.Second,
			HealthCheckInterval:    24 * time.Hour,
			HealthRetryInterval:    time.Hour,
			HealthFailureThreshold: 3,
		},
		domainWorkerID,
	)

	smsRouter, err := platformsms.NewRoutingService(
		platformsms.DefaultRoutingConfig(),
		arkesel.NewProvider(arkesel.NewClient(cfg.Arkesel)),
		celcom.NewProvider(celcom.NewClient(cfg.Celcom)),
		mnotify.NewProvider(mnotify.NewClient(cfg.MNotify)),
	)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS router: %w", err))
	}
	smsSender, err := platformsms.NewService(smsRouter)
	if err != nil {
		return fail(fmt.Errorf("initialize SMS sender: %w", err))
	}
	smsRepository := smsmodule.NewRepositoryWithWebhookEmitter(db, lifecycleEmitter)
	smsConsumer := smsdelivery.NewConsumer(
		messagingClient,
		processedEvents,
		smsdelivery.NewProcessor(smsRepository, smsSender),
		smsdelivery.ConsumerConfig{
			Concurrency:    10,
			AckWait:        2 * time.Minute,
			HandlerTimeout: 45 * time.Second,
			MaxDeliver:     6,
		},
	)
	smsFeedbackRepository := smsfeedback.NewRepository(db)
	smsFeedbackProcessor := smsfeedback.NewProcessor(smsFeedbackRepository)
	smsFeedbackReconciler := smsfeedback.NewReconciler(
		smsFeedbackRepository,
		smsSender,
		smsFeedbackProcessor,
		smsfeedback.ReconcilerConfig{
			BatchSize:       100,
			Concurrency:     10,
			ProviderTimeout: 15 * time.Second,
		},
	)
	smsFeedbackConsumer := smsfeedback.NewConsumer(
		smsFeedbackReconciler,
		smsfeedback.ConsumerConfig{PollInterval: 30 * time.Second},
	)

	verifyCipher, err := authnz.NewSecretCipherKeyring(cfg.EncryptionKeys)
	if err != nil {
		return fail(fmt.Errorf("initialize verification code cipher: %w", err))
	}
	emailAPIService := emailmodule.NewService(
		emailmodule.NewRepository(db),
		emaildelivery.NewQueue(outboxRepository),
		emailmodule.ServiceConfig{
			DefaultFromEmail: cfg.AWS.FromEmail,
			DefaultProvider:  "ses",
			DefaultRegion:    cfg.AWS.Region,
		},
		billingService,
	)
	smsAPIService := smsmodule.NewService(
		smsRepository,
		smsSender,
		smsdelivery.NewQueue(outboxRepository),
		billingService,
	)
	verifyProcessor := verifydispatch.NewProcessor(
		verifydispatch.NewRepository(db),
		verifyCipher,
		verifydispatch.NewEmailChannel(emailAPIService),
		verifydispatch.NewSMSChannel(smsAPIService, "Dugble"),
		events,
	)
	verifyConsumer := verifydispatch.NewConsumer(
		messagingClient,
		processedEvents,
		verifyProcessor,
		verifydispatch.DefaultConsumerConfig(),
	)
	verifyExpiryScanner := verifyexpiry.NewScanner(
		verifyexpiry.NewProcessor(db, events),
		verifyexpiry.DefaultConfig(),
	)
	verifyCleanupWorker := verifycleanup.NewWorker(db, verifycleanup.DefaultConfig())

	outboxRelay := outbox.NewRelay(
		outboxRepository,
		messagingClient,
		outbox.Config{
			PollInterval: 500 * time.Millisecond,
			BatchSize:    100,
			LockTimeout:  30 * time.Second,
		},
	)
	webhookWorkerID := "webhook-delivery-" + uuid.NewString()
	webhookRepository := webhookdelivery.NewRepository(
		db,
		webhookdelivery.RepositoryConfig{AutoDisableAfter: 20},
	)
	webhookProcessor := webhookdelivery.NewProcessor(
		webhookRepository,
		webhookdelivery.NewClient(10*time.Second),
		webhookdelivery.DefaultRetryPolicy(),
		webhookWorkerID,
	)
	webhookConsumer := webhookdelivery.NewConsumer(
		webhookRepository,
		webhookProcessor,
		webhookdelivery.ConsumerConfig{
			PollInterval:  500 * time.Millisecond,
			BatchSize:     50,
			Concurrency:   10,
			LockTimeout:   30 * time.Second,
			HandleTimeout: 15 * time.Second,
		},
		webhookWorkerID,
	)

	application, err := New(
		job{name: "outbox relay", run: outboxRelay.Run},
		job{name: "email delivery consumer", run: emailConsumer.Run},
		job{name: "system email consumer", run: systemEmailConsumer.Run},
		job{name: "email tenant provisioning consumer", run: emailTenantConsumer.Run},
		job{name: "email feedback consumer", run: emailFeedbackConsumer.Run},
		job{name: "email feedback reconciler", run: emailFeedbackReconciler.Run},
		job{name: "email feedback metrics collector", run: emailFeedbackMetricsCollector.Run},
		job{name: "SMS delivery consumer", run: smsConsumer.Run},
		job{name: "SMS feedback reconciler", run: smsFeedbackConsumer.Run},
		job{name: "verification dispatch consumer", run: verifyConsumer.Run},
		job{name: "verification expiry scanner", run: verifyExpiryScanner.Run},
		job{name: "verification cleanup worker", run: verifyCleanupWorker.Run},
		job{name: "webhook delivery consumer", run: webhookConsumer.Run},
		job{name: "sender domain reconciliation consumer", run: domainConsumer.Run},
	)
	if err != nil {
		return fail(fmt.Errorf("create worker application: %w", err))
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
