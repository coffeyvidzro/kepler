package smsrates

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

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Service struct {
	repository store
}

type store interface {
	List(context.Context, int32, int32) ([]SMSRate, error)
	Get(context.Context, uuid.UUID) (SMSRate, error)
	Create(context.Context, CreateInput) (SMSRate, error)
	Close(context.Context, uuid.UUID, time.Time) (SMSRate, error)
}

func NewService(repository store) (*Service, error) {
	if repository == nil {
		return nil, errors.New("backoffice SMS rates repository is required")
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
		return Page{}, apperrors.NewInternal("Unable to list SMS rates", err)
	}
	return Page{SMSRates: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, id string) (SMSRate, error) {
	rateID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return SMSRate{}, apperrors.NewBadRequest("Invalid SMS rate ID")
	}
	item, err := service.repository.Get(ctx, rateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SMSRate{}, apperrors.NewNotFound("SMS rate not found")
		}
		return SMSRate{}, apperrors.NewInternal("Unable to get SMS rate", err)
	}
	return item, nil
}

func (service *Service) Create(ctx context.Context, input CreateInput) (SMSRate, error) {
	input.BillingMarket = strings.ToUpper(strings.TrimSpace(input.BillingMarket))
	input.DestinationCountry = strings.ToUpper(strings.TrimSpace(input.DestinationCountry))
	input.RouteType = strings.ToLower(strings.TrimSpace(input.RouteType))
	input.Tier = strings.ToLower(strings.TrimSpace(input.Tier))
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if !countryCodePattern.MatchString(input.BillingMarket) || !countryCodePattern.MatchString(input.DestinationCountry) {
		return SMSRate{}, apperrors.NewBadRequest("Billing market and destination country must be two-letter ISO country codes")
	}
	if input.RouteType != "local" && input.RouteType != "intl" {
		return SMSRate{}, apperrors.NewBadRequest("Route type must be local or intl")
	}
	if !validTier(input.Tier) {
		return SMSRate{}, apperrors.NewBadRequest("Tier must be growth, scale, or enterprise")
	}
	if !currencyCodePattern.MatchString(input.Currency) {
		return SMSRate{}, apperrors.NewBadRequest("Currency must be a three-letter ISO code")
	}
	if input.CostUnits <= 0 {
		return SMSRate{}, apperrors.NewBadRequest("Cost units must be greater than zero")
	}
	if input.EffectiveFrom.IsZero() {
		return SMSRate{}, apperrors.NewBadRequest("Effective from is required")
	}
	if input.EffectiveUntil != nil && !input.EffectiveUntil.After(input.EffectiveFrom) {
		return SMSRate{}, apperrors.NewBadRequest("Effective until must be after effective from")
	}
	item, err := service.repository.Create(ctx, input)
	if err != nil {
		switch {
		case postgresCode(err, "23P01"):
			return SMSRate{}, apperrors.NewConflict("SMS rate overlaps an existing rate")
		case postgresCode(err, "23503"):
			return SMSRate{}, apperrors.NewBadRequest("Billing market and currency are not configured together")
		default:
			return SMSRate{}, apperrors.NewInternal("Unable to create SMS rate", err)
		}
	}
	return item, nil
}

func (service *Service) Close(ctx context.Context, id string, input CloseInput) (SMSRate, error) {
	rateID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return SMSRate{}, apperrors.NewBadRequest("Invalid SMS rate ID")
	}
	if input.EffectiveUntil.IsZero() {
		return SMSRate{}, apperrors.NewBadRequest("Effective until is required")
	}
	item, err := service.repository.Close(ctx, rateID, input.EffectiveUntil)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SMSRate{}, apperrors.NewInternal("Unable to close SMS rate", err)
	}
	if _, getErr := service.repository.Get(ctx, rateID); errors.Is(getErr, pgx.ErrNoRows) {
		return SMSRate{}, apperrors.NewNotFound("SMS rate not found")
	}
	return SMSRate{}, apperrors.NewConflict("Effective until must shorten the active rate period")
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
