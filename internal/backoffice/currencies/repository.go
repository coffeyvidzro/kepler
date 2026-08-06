package currencies

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) List(ctx context.Context, limit, offset int32) ([]Currency, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("backoffice currencies repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT code, minor_unit, is_enabled
		FROM currencies
		ORDER BY code
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	defer rows.Close()

	items := make([]Currency, 0)
	for rows.Next() {
		var item Currency
		if err := rows.Scan(&item.Code, &item.MinorUnit, &item.IsEnabled); err != nil {
			return nil, fmt.Errorf("scan currency: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currencies: %w", err)
	}
	return items, nil
}

func (repository *Repository) Get(ctx context.Context, code string) (Currency, error) {
	if repository == nil || repository.db == nil {
		return Currency{}, errors.New("backoffice currencies repository is not configured")
	}
	var item Currency
	if err := repository.db.QueryRow(ctx, `
		SELECT code, minor_unit, is_enabled
		FROM currencies
		WHERE code = $1
	`, code).Scan(&item.Code, &item.MinorUnit, &item.IsEnabled); err != nil {
		return Currency{}, fmt.Errorf("get currency: %w", err)
	}
	return item, nil
}

func (repository *Repository) Create(ctx context.Context, input CreateInput) (Currency, error) {
	if repository == nil || repository.db == nil {
		return Currency{}, errors.New("backoffice currencies repository is not configured")
	}
	enabled := true
	if input.IsEnabled != nil {
		enabled = *input.IsEnabled
	}
	var item Currency
	if err := repository.db.QueryRow(ctx, `
		INSERT INTO currencies (code, minor_unit, is_enabled)
		VALUES ($1, $2, $3)
		RETURNING code, minor_unit, is_enabled
	`, input.Code, input.MinorUnit, enabled).Scan(&item.Code, &item.MinorUnit, &item.IsEnabled); err != nil {
		return Currency{}, fmt.Errorf("create currency: %w", err)
	}
	return item, nil
}

func (repository *Repository) SetEnabled(ctx context.Context, code string, enabled bool) (Currency, error) {
	if repository == nil || repository.db == nil {
		return Currency{}, errors.New("backoffice currencies repository is not configured")
	}
	var item Currency
	if err := repository.db.QueryRow(ctx, `
		UPDATE currencies
		SET is_enabled = $2
		WHERE code = $1
		RETURNING code, minor_unit, is_enabled
	`, code, enabled).Scan(&item.Code, &item.MinorUnit, &item.IsEnabled); err != nil {
		return Currency{}, fmt.Errorf("update currency: %w", err)
	}
	return item, nil
}

var _ pgx.Row = currencyRow{}

type currencyRow struct{}

func (currencyRow) Scan(...any) error { return nil }
