package domains

import (
	"context"
	"errors"
	"fmt"

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

const domainProjection = `
	binding.id,
	asset.id,
	COALESCE(asset.team_id::text, ''),
	COALESCE(team.name, ''),
	asset.normalized_identity,
	asset.owner_type,
	asset.status,
	COALESCE(binding.provider, ''),
	binding.provider_account,
	COALESCE(binding.region, ''),
	binding.status,
	COALESCE(binding.provider_status, ''),
	binding.verified,
	binding.health_status,
	binding.attempts,
	binding.consecutive_health_failures,
	COALESCE(binding.last_error, ''),
	binding.last_checked_at,
	binding.next_check_at,
	binding.created_at,
	binding.updated_at`

func (repository *Repository) List(
	ctx context.Context,
	limit int32,
	offset int32,
) ([]Domain, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("backoffice domains repository is not configured")
	}
	rows, err := repository.db.Query(ctx, `
		SELECT `+domainProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset
		  ON asset.id = binding.sender_asset_id
		LEFT JOIN teams AS team
		  ON team.id = asset.team_id
		WHERE asset.channel = 'email'
		ORDER BY binding.created_at DESC, binding.id DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list backoffice domains: %w", err)
	}
	defer rows.Close()

	result := make([]Domain, 0)
	for rows.Next() {
		domain, err := scanDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backoffice domain: %w", err)
		}
		result = append(result, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backoffice domains: %w", err)
	}
	return result, nil
}

func (repository *Repository) Get(ctx context.Context, id uuid.UUID) (Domain, error) {
	if repository == nil || repository.db == nil {
		return Domain{}, errors.New("backoffice domains repository is not configured")
	}
	row := repository.db.QueryRow(ctx, `
		SELECT `+domainProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset
		  ON asset.id = binding.sender_asset_id
		LEFT JOIN teams AS team
		  ON team.id = asset.team_id
		WHERE asset.channel = 'email'
		  AND binding.id = $1
	`, id)
	domain, err := scanDomain(row)
	if err != nil {
		return Domain{}, fmt.Errorf("get backoffice domain: %w", err)
	}
	return domain, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDomain(row rowScanner) (Domain, error) {
	var domain Domain
	var id uuid.UUID
	var assetID uuid.UUID
	var lastCheckedAt pgtype.Timestamptz
	if err := row.Scan(
		&id,
		&assetID,
		&domain.TeamID,
		&domain.TeamName,
		&domain.Name,
		&domain.OwnerType,
		&domain.AssetStatus,
		&domain.Provider,
		&domain.ProviderAccount,
		&domain.Region,
		&domain.Status,
		&domain.ProviderStatus,
		&domain.Verified,
		&domain.HealthStatus,
		&domain.Attempts,
		&domain.ConsecutiveHealthFailures,
		&domain.LastError,
		&lastCheckedAt,
		&domain.NextCheckAt,
		&domain.CreatedAt,
		&domain.UpdatedAt,
	); err != nil {
		return Domain{}, err
	}
	domain.ID = id.String()
	domain.AssetID = assetID.String()
	if lastCheckedAt.Valid {
		value := lastCheckedAt.Time
		domain.LastCheckedAt = &value
	}
	return domain, nil
}
