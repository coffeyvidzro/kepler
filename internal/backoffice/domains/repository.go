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
	domain_record.id,
	domain_record.id,
	domain_record.team_id::text,
	COALESCE(team.name, ''),
	domain_record.normalized_name,
	'team',
	domain_record.status,
	domain_record.provider,
	domain_record.provider_account,
	domain_record.provider_region,
	domain_record.status,
	COALESCE(domain_record.provider_status, ''),
	domain_record.status = 'verified',
	domain_record.health_status,
	domain_record.reconciliation_attempts,
	domain_record.consecutive_health_failures,
	COALESCE(domain_record.last_error, ''),
	domain_record.last_checked_at,
	domain_record.next_check_at,
	domain_record.created_at,
	domain_record.updated_at`

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
		FROM domains AS domain_record
		LEFT JOIN teams AS team ON team.id = domain_record.team_id
		ORDER BY domain_record.created_at DESC, domain_record.id DESC
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
		FROM domains AS domain_record
		LEFT JOIN teams AS team ON team.id = domain_record.team_id
		WHERE domain_record.id = $1
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
