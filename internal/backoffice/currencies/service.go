package currencies

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

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

type store interface {
	List(context.Context, int32, int32) ([]Currency, error)
	Get(context.Context, string) (Currency, error)
	Create(context.Context, CreateInput) (Currency, error)
	SetEnabled(context.Context, string, bool) (Currency, error)
}

type Service struct {
	repository store
}

func NewService(repository store) (*Service, error) {
	if repository == nil {
		return nil, errors.New("backoffice currencies repository is required")
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
		return Page{}, apperrors.NewInternal("Unable to list currencies", err)
	}
	return Page{Currencies: items, Limit: limit, Offset: offset}, nil
}

func (service *Service) Get(ctx context.Context, code string) (Currency, error) {
	code, err := normalizeCode(code)
	if err != nil {
		return Currency{}, err
	}
	item, err := service.repository.Get(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Currency{}, apperrors.NewNotFound("Currency not found")
		}
		return Currency{}, apperrors.NewInternal("Unable to get currency", err)
	}
	return item, nil
}

func (service *Service) Create(ctx context.Context, input CreateInput) (Currency, error) {
	code, err := normalizeCode(input.Code)
	if err != nil {
		return Currency{}, err
	}
	if input.MinorUnit < 0 || input.MinorUnit > 6 {
		return Currency{}, apperrors.NewBadRequest("Minor unit must be between 0 and 6")
	}
	input.Code = code
	item, err := service.repository.Create(ctx, input)
	if err != nil {
		if isPostgresCode(err, "23505") {
			return Currency{}, apperrors.NewConflict("Currency already exists")
		}
		return Currency{}, apperrors.NewInternal("Unable to create currency", err)
	}
	return item, nil
}

func (service *Service) Update(ctx context.Context, code string, input UpdateInput) (Currency, error) {
	code, err := normalizeCode(code)
	if err != nil {
		return Currency{}, err
	}
	item, err := service.repository.SetEnabled(ctx, code, input.IsEnabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Currency{}, apperrors.NewNotFound("Currency not found")
		}
		return Currency{}, apperrors.NewInternal("Unable to update currency", err)
	}
	return item, nil
}

func normalizeCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !currencyCodePattern.MatchString(value) {
		return "", apperrors.NewBadRequest("Currency code must be a three-letter ISO code")
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

func isPostgresCode(err error, code string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == code
}
