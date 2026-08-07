package worker

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	broadcastexecution "github.com/coffeyvidzro/dugble/server/internal/delivery/broadcast"
	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

func newBroadcastExecutionJob(db *pgxpool.Pool, webhookEmitter *platformwebhook.Emitter) job {
	eventEmitter := platformevent.NewEmitter(platformwebhook.NewEventSink(webhookEmitter))
	repository := broadcastmodule.NewRepositoryWithEventEmitter(db, eventEmitter)
	consumer := broadcastexecution.NewConsumer(
		repository,
		broadcastexecution.Config{
			PollInterval: time.Second,
			BatchSize:    100,
		},
	)
	return job{name: "scheduled broadcast execution", run: consumer.Run}
}
