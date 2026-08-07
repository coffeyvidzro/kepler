package contactproperty

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAlreadyExists = errors.New("contact property already exists")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Property, error) {
	fallbackString, fallbackNumber := splitFallback(req.Type, req.FallbackValue)
	var value Property
	var numberText *string
	err := r.db.QueryRow(ctx, `
		INSERT INTO contact_properties (
			team_id, key, value_type, fallback_string, fallback_number
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, team_id, key, value_type, fallback_string, fallback_number::text, created_at, updated_at
	`, teamID, req.Key, req.Type, fallbackString, fallbackNumber).Scan(
		&value.ID,
		&value.TeamID,
		&value.Key,
		&value.Type,
		&fallbackString,
		&numberText,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Property{}, ErrAlreadyExists
		}
		return Property{}, fmt.Errorf("create contact property: %w", err)
	}
	value.FallbackValue, err = joinFallback(value.Type, fallbackString, numberText)
	if err != nil {
		return Property{}, err
	}
	return value, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Property, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, key, value_type, fallback_string, fallback_number::text, created_at, updated_at
		FROM contact_properties
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list contact properties: %w", err)
	}
	defer rows.Close()

	values := make([]Property, 0)
	for rows.Next() {
		value, scanErr := scanProperty(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact properties: %w", err)
	}
	return values, nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Property, error) {
	return scanProperty(r.db.QueryRow(ctx, `
		SELECT id, team_id, key, value_type, fallback_string, fallback_number::text, created_at, updated_at
		FROM contact_properties
		WHERE id = $1 AND team_id = $2
	`, id, teamID))
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, valueType string, fallback any) (Property, error) {
	fallbackString, fallbackNumber := splitFallback(valueType, fallback)
	return scanProperty(r.db.QueryRow(ctx, `
		UPDATE contact_properties
		SET fallback_string = $3,
			fallback_number = $4,
			updated_at = now()
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, key, value_type, fallback_string, fallback_number::text, created_at, updated_at
	`, id, teamID, fallbackString, fallbackNumber))
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Property, error) {
	return scanProperty(r.db.QueryRow(ctx, `
		DELETE FROM contact_properties
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, key, value_type, fallback_string, fallback_number::text, created_at, updated_at
	`, id, teamID))
}

type rowScanner interface{ Scan(...any) error }

func scanProperty(row rowScanner) (Property, error) {
	var value Property
	var fallbackString, numberText *string
	if err := row.Scan(
		&value.ID,
		&value.TeamID,
		&value.Key,
		&value.Type,
		&fallbackString,
		&numberText,
		&value.CreatedAt,
		&value.UpdatedAt,
	); err != nil {
		return Property{}, err
	}
	fallback, err := joinFallback(value.Type, fallbackString, numberText)
	if err != nil {
		return Property{}, err
	}
	value.FallbackValue = fallback
	return value, nil
}

func splitFallback(valueType string, fallback any) (*string, *float64) {
	if fallback == nil {
		return nil, nil
	}
	if valueType == "string" {
		value := fallback.(string)
		return &value, nil
	}
	value, _ := numericValue(fallback)
	return nil, &value
}

func joinFallback(valueType string, stringValue, numberText *string) (any, error) {
	if valueType == "string" {
		if stringValue == nil {
			return nil, nil
		}
		return *stringValue, nil
	}
	if numberText == nil {
		return nil, nil
	}
	value, err := strconv.ParseFloat(*numberText, 64)
	if err != nil {
		return nil, fmt.Errorf("parse contact property fallback: %w", err)
	}
	return value, nil
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

var _ = pgx.ErrNoRows
