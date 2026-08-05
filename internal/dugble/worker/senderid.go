package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	moolresender "github.com/coffeyvidzro/dugble/server/internal/adapters/moolre/sender"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	senderidreconciliation "github.com/coffeyvidzro/dugble/server/internal/delivery/senderid"
	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
)

func newSenderIDReconciliationJob(db *pgxpool.Pool, cfg *config.Config) (job, error) {
	if db == nil || cfg == nil {
		return job{}, errors.New("Sender ID reconciliation dependencies are required")
	}
	if cfg.Moolre.VASKey == "" {
		slog.Warn("Moolre Sender ID reconciliation is disabled because MOOLRE_VAS_KEY is empty")
		return job{
			name: "Sender ID reconciliation",
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}, nil
	}

	provider := moolresender.NewProvider(moolre.NewClient(cfg.Moolre.VASKey))
	consumer, err := senderidreconciliation.NewConsumer(
		senderidmodule.NewRepository(db),
		senderidreconciliation.DefaultConfig(),
		"sender-id-reconciliation-"+uuid.NewString(),
		provider,
	)
	if err != nil {
		return job{}, err
	}
	return job{name: "Sender ID reconciliation", run: consumer.Run}, nil
}
