package billingmarkets

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const projection = `
	market.code,
	market.currency,
	currency.minor_unit,
	market.is_enabled`

func (repository *Repository) List(ctx context.Context, limit, offset int32) ([]BillingMarket, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("backoffice billing markets repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT `+projection+`
		FROM billing_markets AS market
		JOIN currencies AS currency ON currency.code = market.currency
		ORDER BY market.code
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list billing markets: %w", err)
	}
	defer rows.Close()

	items := make([]BillingMarket, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan billing market: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing markets: %w", err)
	}
	return items, nil
}

func (repository *Repository) Get(ctx context.Context, code string) (BillingMarket, error) {
	if repository == nil || repository.db == nil {
		return BillingMarket{}, errors.New("backoffice billing markets repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		SELECT `+projection+`
		FROM billing_markets AS market
		JOIN currencies AS currency ON currency.code = market.currency
		WHERE market.code = $1
	`, code))
}

func (repository *Repository) Create(ctx context.Context, input CreateInput) (BillingMarket, error) {
	if repository == nil || repository.db == nil {
		return BillingMarket{}, errors.New("backoffice billing markets repository is not configured")
	}
	enabled := true
	if input.IsEnabled != nil {
		enabled = *input.IsEnabled
	}
	if _, err := repository.db.Exec(ctx, `
		INSERT INTO billing_markets (code, currency, is_enabled)
		VALUES ($1, $2, $3)
	`, input.Code, input.Currency, enabled); err != nil {
		return BillingMarket{}, fmt.Errorf("create billing market: %w", err)
	}
	return repository.Get(ctx, input.Code)
}

func (repository *Repository) SetEnabled(ctx context.Context, code string, enabled bool) (BillingMarket, error) {
	if repository == nil || repository.db == nil {
		return BillingMarket{}, errors.New("backoffice billing markets repository is not configured")
	}
	result, err := repository.db.Exec(ctx, `
		UPDATE billing_markets
		SET is_enabled = $2
		WHERE code = $1
	`, code, enabled)
	if err != nil {
		return BillingMarket{}, fmt.Errorf("update billing market: %w", err)
	}
	if result.RowsAffected() == 0 {
		return BillingMarket{}, fmt.Errorf("update billing market: no rows affected")
	}
	return repository.Get(ctx, code)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (BillingMarket, error) {
	var item BillingMarket
	if err := row.Scan(&item.Code, &item.Currency, &item.MinorUnit, &item.IsEnabled); err != nil {
		return BillingMarket{}, err
	}
	return item, nil
}
