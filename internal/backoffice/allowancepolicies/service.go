package allowancepolicies

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
var identifierPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type store interface {
	List(context.Context, int32, int32) ([]AllowancePolicy, error)
	Get(context.Context, uuid.UUID) (AllowancePolicy, error)
	Create(context.Context, CreateInput) (AllowancePolicy, error)
	Close(context.Context, uuid.UUID, time.Time) (AllowancePolicy, error)
}

type Service struct{ repository store }

func NewService(repository store) (*Service, error) {
	if repository == nil {
		return nil, errors.New("backoffice allowance policies repository is required")
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
		return Page{}, apperrors.NewInternal("Unable to list allowance policies", err)
	}
	return Page{AllowancePolicies: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, id string) (AllowancePolicy, error) {
	policyID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return AllowancePolicy{}, apperrors.NewBadRequest("Invalid allowance policy ID")
	}
	item, err := service.repository.Get(ctx, policyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AllowancePolicy{}, apperrors.NewNotFound("Allowance policy not found")
		}
		return AllowancePolicy{}, apperrors.NewInternal("Unable to get allowance policy", err)
	}
	return item, nil
}

func (service *Service) Create(ctx context.Context, input CreateInput) (AllowancePolicy, error) {
	input.Product = strings.ToLower(strings.TrimSpace(input.Product))
	input.Meter = strings.ToLower(strings.TrimSpace(input.Meter))
	input.BillingMarket = strings.ToUpper(strings.TrimSpace(input.BillingMarket))
	input.Tier = strings.ToLower(strings.TrimSpace(input.Tier))
	input.Cadence = strings.ToLower(strings.TrimSpace(input.Cadence))
	if input.Cadence == "" {
		input.Cadence = "monthly"
	}
	if !identifierPattern.MatchString(input.Product) || !identifierPattern.MatchString(input.Meter) {
		return AllowancePolicy{}, apperrors.NewBadRequest("Product and meter must use lowercase letters, numbers, or underscores")
	}
	if !marketCodePattern.MatchString(input.BillingMarket) {
		return AllowancePolicy{}, apperrors.NewBadRequest("Billing market must be a two-letter ISO country code")
	}
	if !validTier(input.Tier) {
		return AllowancePolicy{}, apperrors.NewBadRequest("Tier must be growth, scale, or enterprise")
	}
	if input.IncludedQuantity <= 0 {
		return AllowancePolicy{}, apperrors.NewBadRequest("Included quantity must be greater than zero")
	}
	if input.Cadence != "monthly" {
		return AllowancePolicy{}, apperrors.NewBadRequest("Cadence must be monthly")
	}
	if !isUTCMonthBoundary(input.EffectiveFrom) {
		return AllowancePolicy{}, apperrors.NewBadRequest("Effective from must be a UTC month boundary")
	}
	if input.EffectiveUntil != nil {
		if !isUTCMonthBoundary(*input.EffectiveUntil) || !input.EffectiveUntil.After(input.EffectiveFrom) {
			return AllowancePolicy{}, apperrors.NewBadRequest("Effective until must be a later UTC month boundary")
		}
	}
	item, err := service.repository.Create(ctx, input)
	if err != nil {
		switch {
		case postgresCode(err, "23P01"):
			return AllowancePolicy{}, apperrors.NewConflict("Allowance policy overlaps an existing policy")
		case postgresCode(err, "23503"):
			return AllowancePolicy{}, apperrors.NewBadRequest("Billing market is not configured")
		default:
			return AllowancePolicy{}, apperrors.NewInternal("Unable to create allowance policy", err)
		}
	}
	return item, nil
}

func (service *Service) Close(ctx context.Context, id string, input CloseInput) (AllowancePolicy, error) {
	policyID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return AllowancePolicy{}, apperrors.NewBadRequest("Invalid allowance policy ID")
	}
	if !isUTCMonthBoundary(input.EffectiveUntil) {
		return AllowancePolicy{}, apperrors.NewBadRequest("Effective until must be a UTC month boundary")
	}
	item, err := service.repository.Close(ctx, policyID, input.EffectiveUntil)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AllowancePolicy{}, apperrors.NewInternal("Unable to close allowance policy", err)
	}
	if _, getErr := service.repository.Get(ctx, policyID); errors.Is(getErr, pgx.ErrNoRows) {
		return AllowancePolicy{}, apperrors.NewNotFound("Allowance policy not found")
	}
	return AllowancePolicy{}, apperrors.NewConflict("Effective until must shorten the active policy period")
}

func validTier(value string) bool {
	return value == "growth" || value == "scale" || value == "enterprise"
}

func isUTCMonthBoundary(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	utc := value.UTC()
	boundary := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	return utc.Equal(boundary)
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
