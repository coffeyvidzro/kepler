package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	verifyfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/feedback"
	webhookdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/webhook"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/processed"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	webhookmodule "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	"github.com/coffeyvidzro/dugble/server/internal/platform/monitoring/verifymetrics"
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

	processedEvents := processed.NewRepository(db)
	outboxRepository := outbox.NewRepository(db)
	webhookModuleRepository := webhookmodule.NewRepository(db)
	webhookEmitter := platformwebhook.NewEmitter(webhookModuleRepository)
	events := platformevent.NewEmitter(platformwebhook.NewEventSink(webhookEmitter))
	lifecycleEmitter := verifyfeedback.NewEmitter(webhookEmitter, events)
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

	healthServer := &http.Server{
		Addr:              ":8082",
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	healthComponent := Component{
		Name: "health server",
		Run: func(componentCtx context.Context) error {
			go func() {
				<-componentCtx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
					slog.Warn("worker health server shutdown failed", "error", shutdownErr)
				}
			}()
			err := healthServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
	}

	components := []Component{
		healthComponent,
		{Name: "outbox relay", Run: outboxRelay.Run},
		{Name: "email JetStream consumer", Run: emailConsumer.Run},
		{Name: "system email JetStream consumer", Run: systemEmailConsumer.Run},
		{Name: "email tenant provisioning consumer", Run: emailTenantConsumer.Run},
		{Name: "email feedback JetStream consumer", Run: emailFeedbackConsumer.Run},
		{Name: "email feedback database reconciler", Run: emailFeedbackReconciler.Run},
		{Name: "email feedback metrics collector", Run: emailFeedbackMetricsCollector.Run},
		{Name: "SMS JetStream consumer", Run: smsConsumer.Run},
		{Name: "SMS feedback reconciler", Run: smsFeedbackConsumer.Run},
		{Name: "Verify dispatch JetStream consumer", Run: verifyConsumer.Run},
		{Name: "Verify expiry scanner", Run: verifyExpiryScanner.Run},
		{Name: "Verify cleanup worker", Run: verifyCleanupWorker.Run},
		{Name: "webhook delivery consumer", Run: webhookConsumer.Run},
		{Name: "sender domain reconciliation consumer", Run: domainConsumer.Run},
	}
	supervisor, err := NewSupervisor(FailFast, components...)
	if err != nil {
		return fail(fmt.Errorf("create worker supervisor: %w", err))
	}

	healthMux := http.NewServeMux()
	healthMux.Handle("/", NewHealthHandler(db, messagingClient, supervisor))
	healthMux.Handle("GET /metrics", feedbackMetrics)
	healthMux.Handle("GET /metrics/verify", verifymetrics.Default)
	healthServer.Handler = healthMux

	application, err := New(supervisor, healthServer.Addr, "/metrics")
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
