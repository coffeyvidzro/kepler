package billingmarkets

import (
	"context"
	"errors"
	"regexp"
	"strings"

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

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) List(ctx context.Context, input ListInput) (Page, error) {
	limit, offset, err := validatePage(input.Limit, input.Offset)
	if err != nil {
		return Page{}, err
	}
	items, err := service.repository.List(ctx, limit, offset)
	if err != nil {
		return Page{}, apperrors.NewInternal("Unable to list billing markets", err)
	}
	return Page{BillingMarkets: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, code string) (BillingMarket, error) {
	code, err := normalizeMarketCode(code)
	if err != nil {
		return BillingMarket{}, err
	}
	item, err := service.repository.Get(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BillingMarket{}, apperrors.NewNotFound("Billing market not found")
		}
		return BillingMarket{}, apperrors.NewInternal("Unable to get billing market", err)
	}
	return item, nil
}

func (service *Service) Create(ctx context.Context, input CreateInput) (BillingMarket, error) {
	code, err := normalizeMarketCode(input.Code)
	if err != nil {
		return BillingMarket{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if !currencyCodePattern.MatchString(currency) {
		return BillingMarket{}, apperrors.NewBadRequest("Currency must be a three-letter ISO code")
	}
	input.Code = code
	input.Currency = currency
	item, err := service.repository.Create(ctx, input)
	if err != nil {
		if postgresCode(err, "23505") {
			return BillingMarket{}, apperrors.NewConflict("Billing market already exists")
		}
		if postgresCode(err, "23503") {
			return BillingMarket{}, apperrors.NewBadRequest("Currency is not configured")
		}
		return BillingMarket{}, apperrors.NewInternal("Unable to create billing market", err)
	}
	return item, nil
}

func (service *Service) Update(ctx context.Context, code string, input UpdateInput) (BillingMarket, error) {
	code, err := normalizeMarketCode(code)
	if err != nil {
		return BillingMarket{}, err
	}
	item, err := service.repository.SetEnabled(ctx, code, input.IsEnabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BillingMarket{}, apperrors.NewNotFound("Billing market not found")
		}
		return BillingMarket{}, apperrors.NewInternal("Unable to update billing market", err)
	}
	return item, nil
}

func normalizeMarketCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !marketCodePattern.MatchString(value) {
		return "", apperrors.NewBadRequest("Billing market code must be a two-letter ISO country code")
	}
	return value, nil
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
