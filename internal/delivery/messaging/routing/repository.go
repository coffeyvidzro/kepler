package messagingrouting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
)

type DBTX interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type candidateSource interface {
	ListCandidates(context.Context, platformrouting.Request) ([]platformrouting.Candidate, error)
}

type Repository struct {
	email candidateSource
	sms   candidateSource
}

func NewRepository(db DBTX) *Repository {
	return &Repository{
		email: emailDataSource{db: db},
		sms:   smsDataSource{db: db},
	}
}

func Resolve(ctx context.Context, db DBTX, request platformrouting.Request) (platformrouting.Route, error) {
	resolver, err := platformrouting.NewResolver(NewRepository(db), platformrouting.DeterministicStrategy{})
	if err != nil {
		return platformrouting.Route{}, err
	}
	return resolver.Resolve(ctx, request)
}

func ResolveAll(ctx context.Context, db DBTX, request platformrouting.Request) ([]platformrouting.Route, error) {
	resolver, err := platformrouting.NewResolver(NewRepository(db), platformrouting.DeterministicStrategy{})
	if err != nil {
		return nil, err
	}
	return resolver.ResolveAll(ctx, request)
}

func (repository *Repository) ListCandidates(
	ctx context.Context,
	request platformrouting.Request,
) ([]platformrouting.Candidate, error) {
	if repository == nil {
		return nil, fmt.Errorf("messaging routing repository is not configured")
	}
	switch request.Channel {
	case messaging.ChannelEmail:
		if repository.email == nil {
			return nil, fmt.Errorf("email routing data source is not configured")
		}
		return repository.email.ListCandidates(ctx, request)
	case messaging.ChannelSMS:
		if repository.sms == nil {
			return nil, fmt.Errorf("SMS routing data source is not configured")
		}
		return repository.sms.ListCandidates(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported messaging routing channel %q", request.Channel)
	}
}
