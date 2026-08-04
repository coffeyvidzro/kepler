package verify

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type dispatchQueue interface {
	EnqueueVerificationDispatchTx(context.Context, pgx.Tx, verifydispatch.Command) error
}

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
}

type Service struct {
	repository *Repository
	codes      *CodeManager
	dispatch   dispatchQueue
	events     eventEmitter
	abuse      abuseControls
	now        func() time.Time
}

func NewService(repository *Repository, codes *CodeManager, dispatch dispatchQueue, events eventEmitter) *Service {
	return &Service{repository: repository, codes: codes, dispatch: dispatch, events: events, now: time.Now}
}

func (service *Service) WithAbuseControls(controls abuseControls) *Service {
	service.abuse = controls
	return service
}
