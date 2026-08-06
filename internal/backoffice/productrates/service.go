package productrates

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	defaultPageLimit int32 = 50
	maximumPageLimit int32 = 100
)

var marketCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)
var identifierPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type Service struct {
	repository store
}

type store interface {
	List(context.Context, int32, int32) ([]ProductRate, error)
	Get(context.Context, uuid.UUID) (ProductRate, error)
	Create(context.Context, CreateInput) (ProductRate, error)
	Close(context.Context, uuid.UUID, time.Time) (ProductRate, error)
}

func NewService(repository store) (*Service, error) {
	if repository == nil {
		return nil, errors.New("backoffice product rates repository is required")
	}
	return &Service{repository: repository}, nil
}

func (service *Service) List(ctx context.Context, input ListInput) (Page, error) {
	limit, offset, err := validatePage(input.Limit, input.Offset)
	if err != nil {
		return Page{}, err
	}
	items, err := service.repository.List(ctx, limit, offset)
	if err != nil {
		return Page{}, apperrors.NewInternal("Unable to list product rates", err)
	}
	return Page{ProductRates: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, id string) (ProductRate, error) {
	rateID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return ProductRate{}, apperrors.NewBadRequest("Invalid product rate ID")
	}
	item, err := service.repository.Get(ctx, rateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductRate{}, apperrors.NewNotFound("Product rate not found")
		}
		return ProductRate{}, apperrors.NewInternal("Unable to get product rate", err)
	}
	return item, nil
}

func (service *Service) Create(ctx context.Context, input CreateInput) (ProductRate, error) {
	input.Product = strings.ToLower(strings.TrimSpace(input.Product))
	input.Meter = strings.ToLower(strings.TrimSpace(input.Meter))
	input.BillingMarket = strings.ToUpper(strings.TrimSpace(input.BillingMarket))
	input.Tier = strings.ToLower(strings.TrimSpace(input.Tier))
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if !identifierPattern.MatchString(input.Product) || !identifierPattern.MatchString(input.Meter) {
		return ProductRate{}, apperrors.NewBadRequest("Product and meter must use lowercase letters, numbers, or underscores")
	}
	if !marketCodePattern.MatchString(input.BillingMarket) {
		return ProductRate{}, apperrors.NewBadRequest("Billing market must be a two-letter ISO country code")
	}
	if !validTier(input.Tier) {
		return ProductRate{}, apperrors.NewBadRequest("Tier must be growth, scale, or enterprise")
	}
	if !currencyCodePattern.MatchString(input.Currency) {
		return ProductRate{}, apperrors.NewBadRequest("Currency must be a three-letter ISO code")
	}
	if input.CostUnits <= 0 {
		return ProductRate{}, apperrors.NewBadRequest("Cost units must be greater than zero")
	}
	if input.EffectiveFrom.IsZero() {
		return ProductRate{}, apperrors.NewBadRequest("Effective from is required")
	}
	if input.EffectiveUntil != nil && !input.EffectiveUntil.After(input.EffectiveFrom) {
		return ProductRate{}, apperrors.NewBadRequest("Effective until must be after effective from")
	}
	item, err := service.repository.Create(ctx, input)
	if err != nil {
		switch {
		case postgresCode(err, "23P01"):
			return ProductRate{}, apperrors.NewConflict("Product rate overlaps an existing rate")
		case postgresCode(err, "23503"):
			return ProductRate{}, apperrors.NewBadRequest("Billing market and currency are not configured together")
		default:
			return ProductRate{}, apperrors.NewInternal("Unable to create product rate", err)
		}
	}
	return item, nil
}

func (service *Service) Close(ctx context.Context, id string, input CloseInput) (ProductRate, error) {
	rateID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return ProductRate{}, apperrors.NewBadRequest("Invalid product rate ID")
	}
	if input.EffectiveUntil.IsZero() {
		return ProductRate{}, apperrors.NewBadRequest("Effective until is required")
	}
	item, err := service.repository.Close(ctx, rateID, input.EffectiveUntil)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ProductRate{}, apperrors.NewInternal("Unable to close product rate", err)
	}
	if _, getErr := service.repository.Get(ctx, rateID); errors.Is(getErr, pgx.ErrNoRows) {
		return ProductRate{}, apperrors.NewNotFound("Product rate not found")
	}
	return ProductRate{}, apperrors.NewConflict("Effective until must shorten the active rate period")
}

func validTier(value string) bool {
	return value == "growth" || value == "scale" || value == "enterprise"
}

func validatePage(limit, offset int32) (int32, int32, error) {
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

func postgresCode(err error, code string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == code
}
