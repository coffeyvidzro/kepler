package domains

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	defaultPageLimit int32 = 50
	maximumPageLimit int32 = 100
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) List(ctx context.Context, input ListInput) (Page, error) {
	if service == nil || service.repository == nil {
		return Page{}, apperrors.NewInternal("Unable to list domains", errors.New("backoffice domains service is not configured"))
	}
	limit, offset, err := validatePage(input.Limit, input.Offset)
	if err != nil {
		return Page{}, err
	}
	items, err := service.repository.List(ctx, limit, offset)
	if err != nil {
		return Page{}, apperrors.NewInternal("Unable to list domains", err)
	}
	return Page{Domains: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, id string) (Domain, error) {
	if service == nil || service.repository == nil {
		return Domain{}, apperrors.NewInternal("Unable to get domain", errors.New("backoffice domains service is not configured"))
	}
	domainID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return Domain{}, apperrors.NewBadRequest("Invalid domain ID")
	}
	domain, err := service.repository.Get(ctx, domainID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Domain{}, apperrors.NewNotFound("Domain not found")
		}
		return Domain{}, apperrors.NewInternal("Unable to get domain", err)
	}
	return domain, nil
}

func validatePage(limit int32, offset int32) (int32, int32, error) {
	if limit < 0 || limit > maximumPageLimit {
		return 0, 0, apperrors.NewBadRequest("Limit must be between 1 and 100")
	}
	if offset < 0 {
		return 0, 0, apperrors.NewBadRequest("Offset must not be negative")
	}
	if limit == 0 {
		limit = defaultPageLimit
	}
	return limit, offset, nil
}
