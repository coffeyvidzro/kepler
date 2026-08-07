package contactproperty

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

var ErrAlreadyExists = errors.New("contact property already exists")
var ErrCursorNotFound = errors.New("contact property cursor not found")

type Repository struct {
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Property, error) {
	fallbackString, fallbackNumber, err := splitFallback(req.Type, req.FallbackValue)
	if err != nil {
		return Property{}, err
	}
	row, err := r.queries.CreateContactProperty(ctx, dbsqlc.CreateContactPropertyParams{
		TeamID:         teamID,
		Key:            req.Key,
		ValueType:      req.Type,
		FallbackString: fallbackString,
		FallbackNumber: fallbackNumber,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Property{}, ErrAlreadyExists
		}
		return Property{}, fmt.Errorf("create contact property: %w", err)
	}
	return propertyFromSQLC(row)
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, req ListRequest) ([]Property, bool, error) {
	limit := req.Limit + 1
	var (
		rows []dbsqlc.ContactProperty
		err  error
	)

	switch {
	case req.After != "":
		cursorID, parseErr := parseCursor(req.After)
		if parseErr != nil {
			return nil, false, parseErr
		}
		if err := r.ensureCursor(ctx, teamID, cursorID); err != nil {
			return nil, false, err
		}
		rows, err = r.queries.ListContactPropertiesAfter(ctx, dbsqlc.ListContactPropertiesAfterParams{
			ScopeTeamID: teamID, CursorID: cursorID, PageLimit: limit,
		})
	case req.Before != "":
		cursorID, parseErr := parseCursor(req.Before)
		if parseErr != nil {
			return nil, false, parseErr
		}
		if err := r.ensureCursor(ctx, teamID, cursorID); err != nil {
			return nil, false, err
		}
		rows, err = r.queries.ListContactPropertiesBefore(ctx, dbsqlc.ListContactPropertiesBeforeParams{
			ScopeTeamID: teamID, CursorID: cursorID, PageLimit: limit,
		})
	default:
		rows, err = r.queries.ListContactProperties(ctx, dbsqlc.ListContactPropertiesParams{
			TeamID: teamID, PageLimit: limit,
		})
	}
	if err != nil {
		return nil, false, fmt.Errorf("list contact properties: %w", err)
	}

	hasMore := len(rows) > int(req.Limit)
	if hasMore {
		rows = rows[:req.Limit]
	}
	if req.Before != "" {
		slices.Reverse(rows)
	}
	values := make([]Property, 0, len(rows))
	for _, row := range rows {
		value, convertErr := propertyFromSQLC(row)
		if convertErr != nil {
			return nil, false, convertErr
		}
		values = append(values, value)
	}
	return values, hasMore, nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Property, error) {
	row, err := r.queries.GetContactProperty(ctx, dbsqlc.GetContactPropertyParams{ID: id, TeamID: teamID})
	if err != nil {
		return Property{}, err
	}
	return propertyFromSQLC(row)
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, valueType string, fallback any) (Property, error) {
	fallbackString, fallbackNumber, err := splitFallback(valueType, fallback)
	if err != nil {
		return Property{}, err
	}
	row, err := r.queries.UpdateContactPropertyFallback(ctx, dbsqlc.UpdateContactPropertyFallbackParams{
		ID: id, TeamID: teamID, FallbackString: fallbackString, FallbackNumber: fallbackNumber,
	})
	if err != nil {
		return Property{}, err
	}
	return propertyFromSQLC(row)
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Property, error) {
	row, err := r.queries.DeleteContactProperty(ctx, dbsqlc.DeleteContactPropertyParams{ID: id, TeamID: teamID})
	if err != nil {
		return Property{}, err
	}
	return propertyFromSQLC(row)
}

func (r *Repository) ensureCursor(ctx context.Context, teamID, cursorID uuid.UUID) error {
	exists, err := r.queries.ContactPropertyCursorExists(ctx, dbsqlc.ContactPropertyCursorExistsParams{
		CursorID: cursorID,
		TeamID:   teamID,
	})
	if err != nil {
		return fmt.Errorf("validate contact property cursor: %w", err)
	}
	if !exists {
		return ErrCursorNotFound
	}
	return nil
}

func parseCursor(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, ErrCursorNotFound
	}
	return id, nil
}

func propertyFromSQLC(row dbsqlc.ContactProperty) (Property, error) {
	fallback, err := joinFallback(row.ValueType, row.FallbackString, row.FallbackNumber)
	if err != nil {
		return Property{}, err
	}
	return Property{
		ID:            row.ID.String(),
		TeamID:        row.TeamID.String(),
		Key:           row.Key,
		Type:          row.ValueType,
		FallbackValue: fallback,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}, nil
}

func splitFallback(valueType string, fallback any) (*string, pgtype.Numeric, error) {
	if fallback == nil {
		return nil, pgtype.Numeric{}, nil
	}
	if valueType == "string" {
		value := fallback.(string)
		return &value, pgtype.Numeric{}, nil
	}
	value, ok := numericValue(fallback)
	if !ok {
		return nil, pgtype.Numeric{}, errors.New("contact property fallback is not numeric")
	}
	var number pgtype.Numeric
	if err := number.ScanFloat64(pgtype.Float8{Float64: value, Valid: true}); err != nil {
		return nil, pgtype.Numeric{}, fmt.Errorf("encode contact property fallback: %w", err)
	}
	return nil, number, nil
}

func joinFallback(valueType string, stringValue *string, numberValue pgtype.Numeric) (any, error) {
	if valueType == "string" {
		if stringValue == nil {
			return nil, nil
		}
		return *stringValue, nil
	}
	if !numberValue.Valid {
		return nil, nil
	}
	value, err := numberValue.Float64Value()
	if err != nil {
		return nil, fmt.Errorf("parse contact property fallback: %w", err)
	}
	if !value.Valid {
		return nil, nil
	}
	return value.Float64, nil
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}
