package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/database"
	domainreconciliation "github.com/coffeyvidzro/dugble/server/internal/delivery/domain"
	emailfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/send"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	tenantprovision "github.com/coffeyvidzro/dugble/server/internal/delivery/email/tenant"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms"
	verifychannel "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/channel"
	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	verifyexpiry "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/expiry"
	verifyfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/feedback"
	webhookdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/webhooks"
	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	smsintegration "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/arkesel"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/celcom"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/mnotify"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/routing"
	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/processed"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	webhookmodule "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	"github.com/coffeyvidzro/dugble/server/internal/transport/workerhealth"
	workerruntime "github.com/coffeyvidzro/dugble/server/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()
	db, err := database.NewPostgres(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	defer db.Close()
	messagingClient, err := jetstreammessaging.New(startupCtx, cfg.NATSURL, "dugble-worker")
	if err != nil {
		return fmt.Errorf("initialize JetStream: %w", err)
	}
	defer func() {
		if closeErr := messagingClient.Close(); closeErr != nil {
			slog.Warn("close JetStream client", "error", closeErr)
		}
	}()
	if err := messagingClient.Provision(startupCtx, jetstreammessaging.DefaultStreamLimits()); err != nil {
		return fmt.Errorf("provision JetStream topology: %w", err)
	}

	processedEvents := processed.NewRepository(db)
	outboxRepository := outbox.NewRepository(db)
	webhookModuleRepository := webhookmodule.NewRepository(db)
	webhookEmitter := platformwebhook.NewEmitter(webhookModuleRepository)
	events := platformevent.NewEmitter(platformwebhook.NewEventSink(webhookEmitter))
	lifecycleEmitter := verifyfeedback.NewEmitter(webhookEmitter, events)
	billingService := platformbilling.NewService(platformbilling.NewRepository(db))

	emailSender, err := awsses.NewSESSender(startupCtx, cfg.AWS.Region, cfg.AWS.FromEmail, cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SESConfigurationSet)
	if err != nil {
		return fmt.Errorf("initialize SES email sender: %w", err)
	}
	emailConsumer := emaildelivery.NewConsumer(messagingClient, processedEvents, emaildelivery.NewHandler(emaildelivery.NewRepository(db), emailSender), emaildelivery.ConsumerConfig{
		Concurrency: 5, AckWait: 2 * time.Minute, HandlerTimeout: 45 * time.Second, MaxDeliver: 6, RetryPolicy: emaildelivery.DefaultRetryPolicy(),
	})
	systemEmailConsumer := systememail.NewConsumer(messagingClient, processedEvents, emailSender, systememail.ConsumerConfig{
		Concurrency: 3, AckWait: time.Minute, HandlerTimeout: 30 * time.Second, MaxDeliver: 6,
	})
	emailTenantRepository := emailtenant.NewRepository(db)
	emailTenantConsumer := tenantprovision.NewConsumer(
		messagingClient,
		processedEvents,
		tenantprovision.NewHandler(emailTenantRepository, emailSender),
		tenantprovision.Config{Concurrency: 3, AckWait: 2 * time.Minute, HandlerTimeout: 60 * time.Second, RetryBackOff: tenantprovision.DefaultRetryBackOff()},
	)
	feedbackMetrics := emailfeedback.DefaultMetrics
	emailFeedbackRepository := emailfeedback.NewRepositoryWithWebhookEmitter(db, lifecycleEmitter)
	emailFeedbackConsumer := emailfeedback.NewConsumer(messagingClient, processedEvents, emailfeedback.NewHandlerWithMetrics(emailFeedbackRepository, feedbackMetrics), emailfeedback.ConsumerConfig{
		Concurrency: 5, AckWait: time.Minute, HandlerTimeout: 30 * time.Second, MaxDeliver: 6, RetryPolicy: emailfeedback.DefaultRetryPolicy(),
	})
	emailFeedbackReconciler := emailfeedback.NewObservedReconciler(emailFeedbackRepository, emailfeedback.ReconcilerConfig{
		PollInterval: 5 * time.Second, BatchSize: 25, Concurrency: 5, LeaseDuration: 2 * time.Minute, HandleTimeout: 30 * time.Second,
	}, feedbackMetrics)
	emailFeedbackMetricsCollector := emailfeedback.NewMetricsCollector(db, feedbackMetrics, 15*time.Second)
	domainRepository := domainmodule.NewRepository(db)
	domainService := domainmodule.NewService(domainRepository, emailSender, platformemail.NewNetDNSVerifier())
	domainWorkerID := "sender-domain-reconciliation-" + uuid.NewString()
	domainConsumer := domainreconciliation.NewConsumer(domainRepository, domainService, domainreconciliation.Config{
		PollInterval: 30 * time.Second, BatchSize: 25, Concurrency: 5, LockTimeout: 2 * time.Minute,
		CheckTimeout: 20 * time.Second, HealthCheckInterval: 24 * time.Hour, HealthRetryInterval: time.Hour, HealthFailureThreshold: 3,
	}, domainWorkerID)

	smsRouter, err := routing.NewService(
		routing.DefaultConfig(),
		arkesel.NewProvider(arkesel.NewClient(cfg.Arkesel)),
		celcom.NewProvider(celcom.NewClient(cfg.Celcom)),
		mnotify.NewProvider(mnotify.NewClient(cfg.MNotify)),
	)
	if err != nil {
		return fmt.Errorf("initialize SMS router: %w", err)
	}
	smsSender, err := smsintegration.NewService(smsRouter)
	if err != nil {
		return fmt.Errorf("initialize SMS sender: %w", err)
	}
	smsRepository := smsmodule.NewRepositoryWithWebhookEmitter(db, lifecycleEmitter)
	smsHandler := smsdelivery.NewHandler(smsRepository, smsSender)
	smsConsumer := smsdelivery.NewConsumer(messagingClient, processedEvents, smsHandler, smsdelivery.ConsumerConfig{Concurrency: 10, AckWait: 2 * time.Minute, HandlerTimeout: 45 * time.Second, MaxDeliver: 6})

	verifyCipher, err := authnz.NewSecretCipherKeyring(cfg.EncryptionKeys)
	if err != nil {
		return fmt.Errorf("initialize verification code cipher: %w", err)
	}
	emailAPIService := emailmodule.NewService(
		emailmodule.NewRepository(db),
		emaildelivery.NewQueue(outboxRepository),
		emailmodule.ServiceConfig{DefaultFromEmail: cfg.AWS.FromEmail, DefaultProvider: "ses", DefaultRegion: cfg.AWS.Region},
		billingService,
	)
	smsAPIService := smsmodule.NewService(smsRepository, smsSender, smsdelivery.NewQueue(outboxRepository), billingService)
	verifyHandler := verifydispatch.NewHandler(
		verifydispatch.NewRepository(db),
		verifyCipher,
		verifychannel.NewEmail(emailAPIService),
		verifychannel.NewSMS(smsAPIService, "Dugble"),
		events,
	)
	verifyConsumer := verifydispatch.NewConsumer(messagingClient, processedEvents, verifyHandler, verifydispatch.DefaultConsumerConfig())
	verifyExpiryConsumer := verifyexpiry.NewConsumer(verifyexpiry.NewRepository(db, events), verifyexpiry.DefaultConfig())

	outboxRelay := outbox.NewRelay(outboxRepository, messagingClient, outbox.Config{PollInterval: 500 * time.Millisecond, BatchSize: 100, LockTimeout: 30 * time.Second})
	webhookWorkerID := "webhook-delivery-" + uuid.NewString()
	webhookRepository := webhookdelivery.NewRepository(db, webhookdelivery.RepositoryConfig{AutoDisableAfter: 20})
	webhookHandler := webhookdelivery.NewHandler(webhookRepository, webhookdelivery.NewClient(10*time.Second), webhookdelivery.DefaultRetryPolicy(), webhookWorkerID)
	webhookConsumer := webhookdelivery.NewConsumer(webhookRepository, webhookHandler, webhookdelivery.ConsumerConfig{PollInterval: 500 * time.Millisecond, BatchSize: 50, Concurrency: 10, LockTimeout: 30 * time.Second, HandleTimeout: 15 * time.Second}, webhookWorkerID)

	var supervisor *workerruntime.Supervisor
	healthServer := &http.Server{Addr: ":8082", ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	healthComponent := workerruntime.Component{Name: "health server", Run: func(componentCtx context.Context) error {
		go func() {
			<-componentCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
				slog.Warn("worker health server shutdown failed", "error", shutdownErr)
			}
		}()
		err := healthServer.ListenAndServe()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}}
	components := []workerruntime.Component{
		healthComponent,
		{Name: "outbox relay", Run: outboxRelay.Run},
		{Name: "email JetStream consumer", Run: emailConsumer.Run},
		{Name: "system email JetStream consumer", Run: systemEmailConsumer.Run},
		{Name: "email tenant provisioning consumer", Run: emailTenantConsumer.Run},
		{Name: "email feedback JetStream consumer", Run: emailFeedbackConsumer.Run},
		{Name: "email feedback database reconciler", Run: emailFeedbackReconciler.Run},
		{Name: "email feedback metrics collector", Run: emailFeedbackMetricsCollector.Run},
		{Name: "SMS JetStream consumer", Run: smsConsumer.Run},
		{Name: "Verify dispatch JetStream consumer", Run: verifyConsumer.Run},
		{Name: "Verify expiry database consumer", Run: verifyExpiryConsumer.Run},
		{Name: "webhook delivery consumer", Run: webhookConsumer.Run},
		{Name: "sender domain reconciliation consumer", Run: domainConsumer.Run},
	}
	supervisor, err = workerruntime.NewSupervisor(workerruntime.FailFast, components...)
	if err != nil {
		return fmt.Errorf("create worker supervisor: %w", err)
	}
	healthMux := http.NewServeMux()
	healthMux.Handle("/", workerhealth.NewHandler(db, messagingClient, supervisor).Routes())
	healthMux.Handle("GET /metrics", feedbackMetrics)
	healthServer.Handler = healthMux
	slog.Info("worker starting", "failure_policy", supervisor.Policy(), "health_address", healthServer.Addr, "metrics_path", "/metrics")
	if err := supervisor.Run(ctx, 30*time.Second); err != nil {
		return err
	}
	slog.Info("worker stopped")
	return nil
}
