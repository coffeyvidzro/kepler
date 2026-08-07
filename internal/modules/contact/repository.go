package contact

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

var (
	ErrAlreadyExists        = errors.New("contact already exists")
	ErrUnknownProperty      = errors.New("unknown contact property")
	ErrPropertyTypeMismatch = errors.New("contact property type mismatch")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Contact, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Contact{}, fmt.Errorf("begin contact creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result Contact
	err = tx.QueryRow(ctx, `
		INSERT INTO contacts (team_id, email, first_name, last_name, unsubscribed)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
	`, teamID, req.Email, req.FirstName, req.LastName, req.Unsubscribed).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Contact{}, ErrAlreadyExists
		}
		return Contact{}, fmt.Errorf("create contact: %w", err)
	}

	contactID, err := uuid.Parse(result.ID)
	if err != nil {
		return Contact{}, fmt.Errorf("parse created contact id: %w", err)
	}
	if err := replaceProperties(ctx, tx, teamID, contactID, req.Properties); err != nil {
		return Contact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Contact{}, fmt.Errorf("commit contact creation: %w", err)
	}
	result.Properties = cloneProperties(req.Properties)
	return result, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Contact, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
		FROM contacts
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]Contact, 0)
	for rows.Next() {
		var value Contact
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Email, &value.FirstName, &value.LastName, &value.Unsubscribed, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}

	for i := range contacts {
		contactID, parseErr := uuid.Parse(contacts[i].ID)
		if parseErr != nil {
			return nil, fmt.Errorf("parse contact id: %w", parseErr)
		}
		properties, loadErr := loadProperties(ctx, r.db, teamID, contactID)
		if loadErr != nil {
			return nil, loadErr
		}
		contacts[i].Properties = properties
	}
	return contacts, nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Contact, error) {
	var result Contact
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
		FROM contacts
		WHERE id = $1 AND team_id = $2
	`, id, teamID).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return Contact{}, err
	}
	result.Properties, err = loadProperties(ctx, r.db, teamID, id)
	if err != nil {
		return Contact{}, err
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, req CreateRequest) (Contact, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Contact{}, fmt.Errorf("begin contact update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result Contact
	err = tx.QueryRow(ctx, `
		UPDATE contacts
		SET email = $3,
			first_name = $4,
			last_name = $5,
			unsubscribed = $6,
			updated_at = now()
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
	`, id, teamID, req.Email, req.FirstName, req.LastName, req.Unsubscribed).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Contact{}, ErrAlreadyExists
		}
		return Contact{}, err
	}
	if err := replaceProperties(ctx, tx, teamID, id, req.Properties); err != nil {
		return Contact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Contact{}, fmt.Errorf("commit contact update: %w", err)
	}
	result.Properties = cloneProperties(req.Properties)
	return result, nil
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Contact, error) {
	var result Contact
	err := r.db.QueryRow(ctx, `
		DELETE FROM contacts
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, email, first_name, last_name, unsubscribed, created_at, updated_at
	`, id, teamID).Scan(
		&result.ID,
		&result.TeamID,
		&result.Email,
		&result.FirstName,
		&result.LastName,
		&result.Unsubscribed,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return Contact{}, err
	}
	result.Properties = map[string]any{}
	return result, nil
}

func replaceProperties(ctx context.Context, tx pgx.Tx, teamID, contactID uuid.UUID, properties map[string]any) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contact_property_values WHERE team_id = $1 AND contact_id = $2`, teamID, contactID); err != nil {
		return fmt.Errorf("clear contact properties: %w", err)
	}
	for key, value := range properties {
		var propertyID uuid.UUID
		var valueType string
		err := tx.QueryRow(ctx, `
			SELECT id, value_type
			FROM contact_properties
			WHERE team_id = $1 AND key = $2
		`, teamID, key).Scan(&propertyID, &valueType)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUnknownProperty, key)
		}
		if err != nil {
			return fmt.Errorf("get contact property %q: %w", key, err)
		}

		switch valueType {
		case "string":
			stringValue, ok := value.(string)
			if !ok {
				return fmt.Errorf("%w: %s must be a string", ErrPropertyTypeMismatch, key)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO contact_property_values (
					team_id, contact_id, contact_property_id, value_type, string_value
				) VALUES ($1, $2, $3, 'string', $4)
			`, teamID, contactID, propertyID, stringValue)
		case "number":
			numberValue, ok := numericValue(value)
			if !ok {
				return fmt.Errorf("%w: %s must be a number", ErrPropertyTypeMismatch, key)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO contact_property_values (
					team_id, contact_id, contact_property_id, value_type, number_value
				) VALUES ($1, $2, $3, 'number', $4)
			`, teamID, contactID, propertyID, numberValue)
		default:
			return fmt.Errorf("unsupported contact property type %q", valueType)
		}
		if err != nil {
			return fmt.Errorf("store contact property %q: %w", key, err)
		}
	}
	return nil
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadProperties(ctx context.Context, db queryer, teamID, contactID uuid.UUID) (map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT cp.key, cpv.value_type, cpv.string_value, cpv.number_value::text
		FROM contact_property_values AS cpv
		JOIN contact_properties AS cp
		  ON cp.id = cpv.contact_property_id
		 AND cp.team_id = cpv.team_id
		WHERE cpv.team_id = $1 AND cpv.contact_id = $2
		ORDER BY cp.key
	`, teamID, contactID)
	if err != nil {
		return nil, fmt.Errorf("load contact properties: %w", err)
	}
	defer rows.Close()

	properties := make(map[string]any)
	for rows.Next() {
		var key, valueType string
		var stringValue, numberText *string
		if err := rows.Scan(&key, &valueType, &stringValue, &numberText); err != nil {
			return nil, fmt.Errorf("scan contact property: %w", err)
		}
		if valueType == "string" && stringValue != nil {
			properties[key] = *stringValue
		}
		if valueType == "number" && numberText != nil {
			numberValue, parseErr := strconv.ParseFloat(*numberText, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("parse contact property %q: %w", key, parseErr)
			}
			properties[key] = numberValue
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact properties: %w", err)
	}
	return properties, nil
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

func cloneProperties(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}
