package server

import (
	"fmt"

	"github.com/arcjet/arcjet-go"
	"github.com/jackc/pgx/v5/pgxpool"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	leamoutsms "github.com/coffeyvidzro/dugble/server/internal/adapters/leamout/sms"
	mnotifyadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	mnotifysms "github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify/sms"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	moolresms "github.com/coffeyvidzro/dugble/server/internal/adapters/moolre/sms"
	runnagesms "github.com/coffeyvidzro/dugble/server/internal/adapters/runnage/sms"
	arcjetadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/security/arcjet"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	providersns "github.com/coffeyvidzro/dugble/server/internal/transport/provider/aws/sns"
)

func newArcjetClient(cfg *config.Config) (*arcjet.Client, error) {
	return arcjetadapter.New(cfg.ArcjetKey)
}

func newEmailClient(cfg *config.Config) (*awsses.Client, error) {
	return awsses.NewClient(
		cfg.AWS.Region,
		cfg.AWS.FromEmail,
		cfg.AWS.AccessKey,
		cfg.AWS.SecretKey,
		cfg.AWS.SESTransactionalConfigurationSet,
	)
}

func newSystemEmailQueue(cfg *config.Config, outboxRepository *outbox.Repository) *systememail.Queue {
	return systememail.NewQueue(outboxRepository, platformemail.Message{
		Provider:         awsses.ProviderSES,
		Region:           cfg.AWS.Region,
		Stream:           "transactional",
		ConfigurationSet: cfg.AWS.SESTransactionalConfigurationSet,
		SESTenantName:    cfg.AWS.SESTenantName,
	})
}

func newProviderSNSHandler(
	cfg *config.Config,
	db *pgxpool.Pool,
	outboxRepository *outbox.Repository,
) *providersns.Handler {
	if len(cfg.AWS.SNSTopicARNs) == 0 {
		return nil
	}

	certificateLoader := awssns.NewHTTPCertificateLoader(nil)
	verifier := awssns.NewVerifier(cfg.AWS.SNSTopicARNs, certificateLoader)
	confirmer := awssns.NewConfirmer(awssns.NewHTTPConfirmSubscriptionClient(nil))
	ingestor := feedback.NewRepository(db, outboxRepository)
	return providersns.NewHandler(verifier, confirmer, ingestor)
}

func newSMSSender(cfg *config.Config) (*platformsms.Service, error) {
	smsRouter, err := platformsms.NewRoutingService(
		platformsms.DefaultRoutingConfig(),
		mnotifysms.NewProvider(mnotifyadapter.NewClient(cfg.MNotify.APIKey)),
		moolresms.NewProvider(moolre.NewClient(cfg.Moolre.VASKey)),
		leamoutsms.NewProvider(),
		runnagesms.NewProvider(),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize SMS router: %w", err)
	}

	smsSender, err := platformsms.NewService(smsRouter)
	if err != nil {
		return nil, fmt.Errorf("initialize SMS sender: %w", err)
	}
	return smsSender, nil
}
