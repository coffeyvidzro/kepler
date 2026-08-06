package productrates

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const projection = `
	id,
	product,
	meter,
	billing_market,
	tier,
	currency,
	cost_units,
	effective_from,
	effective_until,
	created_at`

func (repository *Repository) List(ctx context.Context, limit, offset int32) ([]ProductRate, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("backoffice product rates repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT `+projection+`
		FROM product_rates
		ORDER BY effective_from DESC, created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list product rates: %w", err)
	}
	defer rows.Close()
	items := make([]ProductRate, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product rate: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product rates: %w", err)
	}
	return items, nil
}

func (repository *Repository) Get(ctx context.Context, id uuid.UUID) (ProductRate, error) {
	if repository == nil || repository.db == nil {
		return ProductRate{}, errors.New("backoffice product rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `SELECT `+projection+` FROM product_rates WHERE id = $1`, id))
}

func (repository *Repository) Create(ctx context.Context, input CreateInput) (ProductRate, error) {
	if repository == nil || repository.db == nil {
		return ProductRate{}, errors.New("backoffice product rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		INSERT INTO product_rates (
			product, meter, billing_market, tier, currency,
			cost_units, effective_from, effective_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+projection+`
	`, input.Product, input.Meter, input.BillingMarket, input.Tier, input.Currency, input.CostUnits, input.EffectiveFrom, input.EffectiveUntil))
}

func (repository *Repository) Close(ctx context.Context, id uuid.UUID, effectiveUntil time.Time) (ProductRate, error) {
	if repository == nil || repository.db == nil {
		return ProductRate{}, errors.New("backoffice product rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		UPDATE product_rates
		SET effective_until = $2
		WHERE id = $1
		  AND $2 > effective_from
		  AND (effective_until IS NULL OR $2 < effective_until)
		RETURNING `+projection+`
	`, id, effectiveUntil))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (ProductRate, error) {
	var item ProductRate
	var id uuid.UUID
	var effectiveUntil pgtype.Timestamptz
	if err := row.Scan(
		&id,
		&item.Product,
		&item.Meter,
		&item.BillingMarket,
		&item.Tier,
		&item.Currency,
		&item.CostUnits,
		&item.EffectiveFrom,
		&effectiveUntil,
		&item.CreatedAt,
	); err != nil {
		return ProductRate{}, err
	}
	item.ID = id.String()
	if effectiveUntil.Valid {
		value := effectiveUntil.Time
		item.EffectiveUntil = &value
	}
	return item, nil
}
