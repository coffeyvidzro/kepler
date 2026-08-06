package smsrates

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
	billing_market,
	destination_country,
	route_type,
	tier,
	currency,
	cost_units,
	effective_from,
	effective_until,
	created_at`

func (repository *Repository) List(ctx context.Context, limit, offset int32) ([]SMSRate, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("backoffice SMS rates repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT `+projection+`
		FROM sms_rates
		ORDER BY effective_from DESC, created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list SMS rates: %w", err)
	}
	defer rows.Close()

	items := make([]SMSRate, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan SMS rate: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SMS rates: %w", err)
	}
	return items, nil
}

func (repository *Repository) Get(ctx context.Context, id uuid.UUID) (SMSRate, error) {
	if repository == nil || repository.db == nil {
		return SMSRate{}, errors.New("backoffice SMS rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		SELECT `+projection+`
		FROM sms_rates
		WHERE id = $1
	`, id))
}

func (repository *Repository) Create(ctx context.Context, input CreateInput) (SMSRate, error) {
	if repository == nil || repository.db == nil {
		return SMSRate{}, errors.New("backoffice SMS rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		INSERT INTO sms_rates (
			billing_market,
			destination_country,
			route_type,
			tier,
			currency,
			cost_units,
			effective_from,
			effective_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+projection+`
	`,
		input.BillingMarket,
		input.DestinationCountry,
		input.RouteType,
		input.Tier,
		input.Currency,
		input.CostUnits,
		input.EffectiveFrom,
		input.EffectiveUntil,
	))
}

func (repository *Repository) Close(ctx context.Context, id uuid.UUID, effectiveUntil time.Time) (SMSRate, error) {
	if repository == nil || repository.db == nil {
		return SMSRate{}, errors.New("backoffice SMS rates repository is not configured")
	}
	return scan(repository.db.QueryRow(ctx, `
		UPDATE sms_rates
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

func scan(row rowScanner) (SMSRate, error) {
	var item SMSRate
	var id uuid.UUID
	var effectiveUntil pgtype.Timestamptz
	if err := row.Scan(
		&id,
		&item.BillingMarket,
		&item.DestinationCountry,
		&item.RouteType,
		&item.Tier,
		&item.Currency,
		&item.CostUnits,
		&item.EffectiveFrom,
		&effectiveUntil,
		&item.CreatedAt,
	); err != nil {
		return SMSRate{}, err
	}
	item.ID = id.String()
	if effectiveUntil.Valid {
		value := effectiveUntil.Time
		item.EffectiveUntil = &value
	}
	return item, nil
}
