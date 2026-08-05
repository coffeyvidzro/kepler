from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file_path = Path(path)
    source = file_path.read_text()
    if source.count(old) != 1:
        raise SystemExit(f"expected one match in {path}: {old[:80]!r}")
    file_path.write_text(source.replace(old, new, 1))


migration = Path("migrations/008_create_sender_trust_plane.sql")
source = migration.read_text()
marker = "-- Compatibility views keep the existing application surface operational while\n"
if marker not in source:
    raise SystemExit("sender compatibility section was not found")
migration.write_text(source[: source.index(marker)].rstrip() + "\n")

replace_once(
    "migrations/010_create_sms_messages.sql",
    "    -- Compatibility name retained at the API boundary. The value is the\n"
    "    -- country/provider-specific sender_provider_bindings.id.\n"
    "    sender_id UUID REFERENCES sender_provider_bindings(id) ON DELETE SET NULL,",
    "    sender_provider_binding_id UUID REFERENCES sender_provider_bindings(id) ON DELETE SET NULL,",
)
replace_once(
    "migrations/010_create_sms_messages.sql",
    "CREATE INDEX IF NOT EXISTS idx_sms_messages_provider_message",
    "CREATE INDEX IF NOT EXISTS idx_sms_messages_sender_provider_binding\n"
    "    ON sms_messages (sender_provider_binding_id)\n"
    "    WHERE sender_provider_binding_id IS NOT NULL;\n\n"
    "CREATE INDEX IF NOT EXISTS idx_sms_messages_provider_message",
)
replace_once(
    "migrations/013_create_email_messages.sql",
    "    -- Compatibility name retained at the API boundary. The value is the\n"
    "    -- region/provider-specific sender_provider_bindings.id.\n"
    "    sender_domain_id UUID REFERENCES sender_provider_bindings(id) ON DELETE SET NULL,",
    "    sender_provider_binding_id UUID REFERENCES sender_provider_bindings(id) ON DELETE SET NULL,",
)
replace_once(
    "migrations/013_create_email_messages.sql",
    "CREATE INDEX IF NOT EXISTS idx_email_messages_sender_domain\n"
    "    ON email_messages (sender_domain_id)\n"
    "    WHERE sender_domain_id IS NOT NULL;",
    "CREATE INDEX IF NOT EXISTS idx_email_messages_sender_provider_binding\n"
    "    ON email_messages (sender_provider_binding_id)\n"
    "    WHERE sender_provider_binding_id IS NOT NULL;",
)

for path in [
    "migrations/015_enforce_tenant_invariants.sql",
    "migrations/025_enforce_customer_email_tenant_routes.sql",
    "internal/database/queries/email_messages.sql",
    "internal/database/queries/sms_messages.sql",
    "internal/delivery/email/outbound/repository.go",
]:
    file_path = Path(path)
    source = file_path.read_text()
    source = source.replace("sender_domain_id", "sender_provider_binding_id")
    if path != "internal/database/queries/sms_messages.sql":
        source = source.replace("sender_id", "sender_provider_binding_id")
    file_path.write_text(source)

replace_once(
    "internal/database/queries/sms_messages.sql",
    "    sender_id,",
    "    sender_provider_binding_id,",
)
replace_once(
    "internal/database/queries/sms_messages.sql",
    "    sqlc.narg(sender_id),",
    "    sqlc.narg(sender_provider_binding_id),",
)
replace_once(
    "internal/database/queries/sms_messages.sql",
    "-- name: FindApprovedSMSSender :one\n"
    "SELECT id\n"
    "FROM sender_ids\n"
    "WHERE team_id = sqlc.arg(team_id)\n"
    "  AND lower(name) = lower(sqlc.arg(name))\n"
    "  AND status = 'approved'\n"
    "ORDER BY created_at DESC\n"
    "LIMIT 1;",
    "-- name: FindApprovedSMSSender :one\n"
    "SELECT binding.id\n"
    "FROM sender_provider_bindings AS binding\n"
    "JOIN sender_assets AS asset\n"
    "  ON asset.id = binding.sender_asset_id\n"
    "JOIN sender_asset_grants AS grant_record\n"
    "  ON grant_record.sender_asset_id = asset.id\n"
    " AND grant_record.team_id = sqlc.arg(team_id)\n"
    " AND grant_record.channel = 'sms'\n"
    " AND grant_record.status = 'active'\n"
    "WHERE asset.channel = 'sms'\n"
    "  AND asset.normalized_identity = lower(trim(sqlc.arg(name)))\n"
    "  AND asset.status = 'active'\n"
    "  AND binding.status = 'active'\n"
    "  AND binding.verified\n"
    "ORDER BY binding.created_at DESC\n"
    "LIMIT 1;",
)

replace_once(
    "internal/modules/email/repository.go",
    "\t\tSenderDomainID:   req.SenderDomainID,",
    "\t\tSenderProviderBindingID: req.SenderDomainID,",
)
replace_once(
    "internal/modules/sms/repository.go",
    "\t\tSenderID:           params.SenderID,",
    "\t\tSenderProviderBindingID: params.SenderID,",
)
Path("internal/modules/sms/repository.go").write_text(
    Path("internal/modules/sms/repository.go")
    .read_text()
    .replace("row.SenderID", "row.SenderProviderBindingID")
)

email_repository = Path("internal/modules/email/repository.go")
source = email_repository.read_text()
start = source.index("func (r *Repository) ResolveSenderDomain(")
end = source.index("\nfunc (r *Repository) CancelTx", start)
replacement = r'''func (r *Repository) ResolveSenderDomain(ctx context.Context, teamID uuid.UUID, domainName string) (SenderDomainRoute, error) {
	var route SenderDomainRoute
	var disabledAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT binding.id,
			CASE binding.provider WHEN 'ses' THEN 'aws_ses' ELSE binding.provider END,
			COALESCE(binding.region, ''),
			CASE binding.status
				WHEN 'active' THEN 'verified'
				WHEN 'rejected' THEN 'failed'
				ELSE binding.status
			END,
			binding.health_status,
			binding.disabled_at
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset
		  ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $1
		 AND grant_record.channel = 'email'
		 AND grant_record.status = 'active'
		WHERE asset.channel = 'email'
		  AND asset.normalized_identity = lower(trim($2))
		ORDER BY binding.created_at DESC
		LIMIT 1
	`, teamID, domainName).Scan(
		&route.ID,
		&route.Provider,
		&route.Region,
		&route.Status,
		&route.HealthStatus,
		&disabledAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SenderDomainRoute{}, ErrSenderDomainNotFound
	}
	if err != nil {
		return SenderDomainRoute{}, fmt.Errorf("resolve sender domain: %w", err)
	}
	if route.HealthStatus == "degraded" {
		route.Status = "degraded"
	}
	route.Disabled = disabledAt != nil
	return route, nil
}
'''
email_repository.write_text(source[:start] + replacement + source[end:])

Path("internal/modules/senderid/repository.go").write_text(r'''package senderid

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSenderIDAlreadyExists = errors.New("sender id already exists")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

const senderIDProjection = `
	binding.id,
	asset.team_id,
	asset.identity,
	COALESCE(binding.country_code::text, ''),
	COALESCE(asset.purpose, ''),
	CASE binding.status
		WHEN 'active' THEN 'approved'
		WHEN 'disabled' THEN 'inactive'
		WHEN 'failed' THEN 'rejected'
		ELSE binding.status
	END,
	binding.provider,
	binding.rejection_reason,
	binding.verified_at,
	binding.rejected_at,
	binding.suspended_at,
	asset.created_by,
	binding.created_at,
	binding.updated_at`

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	name string,
	countryCode string,
	purpose string,
	provider *string,
	createdBy uuid.UUID,
) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}

	normalizedName := strings.ToLower(strings.TrimSpace(name))
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	var normalizedProvider *string
	if provider != nil {
		value := strings.ToLower(strings.TrimSpace(*provider))
		if value != "" {
			normalizedProvider = &value
		}
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderID{}, fmt.Errorf("begin sender id creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var assetID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sender_assets (
			owner_type, team_id, channel, identity, normalized_identity,
			purpose, status, health_status, created_by
		)
		SELECT 'team', team.id, 'sms', $2, $3, NULLIF(trim($4), ''),
			'pending', 'unknown', $5
		FROM teams AS team
		WHERE team.id = $1
		  AND team.status = 'active'
		ON CONFLICT (team_id, channel, normalized_identity)
			WHERE owner_type = 'team'
		DO UPDATE SET
			identity = EXCLUDED.identity,
			purpose = EXCLUDED.purpose,
			updated_at = now()
		RETURNING id
	`, teamID, strings.TrimSpace(name), normalizedName, purpose, createdBy).Scan(&assetID)
	if err != nil {
		return SenderID{}, fmt.Errorf("create sender asset: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sender_asset_grants (
			team_id, sender_asset_id, channel, status, granted_by
		) VALUES ($1, $2, 'sms', 'active', $3)
		ON CONFLICT (team_id, sender_asset_id)
		DO UPDATE SET
			status = 'active',
			revoked_at = NULL,
			updated_at = now()
	`, teamID, assetID, createdBy); err != nil {
		return SenderID{}, fmt.Errorf("grant sender asset: %w", err)
	}

	var bindingID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sender_provider_bindings (
			sender_asset_id, provider, country_code, status, health_status
		) VALUES ($1, $2, $3, 'pending', 'unknown')
		RETURNING id
	`, assetID, normalizedProvider, countryCode).Scan(&bindingID)
	if err != nil {
		if isUniqueViolation(err) {
			return SenderID{}, ErrSenderIDAlreadyExists
		}
		return SenderID{}, fmt.Errorf("create sender provider binding: %w", err)
	}

	sender, err := getSenderID(ctx, tx, bindingID, teamID, false)
	if err != nil {
		return SenderID{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderID{}, fmt.Errorf("commit sender id creation: %w", err)
	}
	return sender, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderID, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sender id repository is not configured")
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+senderIDProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $1
		 AND grant_record.channel = 'sms'
		 AND grant_record.status = 'active'
		JOIN teams AS team ON team.id = grant_record.team_id
		WHERE asset.channel = 'sms'
		  AND team.status = 'active'
		ORDER BY binding.created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list sender ids: %w", err)
	}
	defer rows.Close()

	senders := make([]SenderID, 0)
	for rows.Next() {
		sender, err := scanSenderID(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sender id: %w", err)
		}
		senders = append(senders, sender)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sender ids: %w", err)
	}
	return senders, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}
	return getSenderID(ctx, r.db, id, teamID, false)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderID{}, fmt.Errorf("begin sender id deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sender, err := getSenderID(ctx, tx, id, teamID, true)
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id for deletion: %w", err)
	}
	var assetID uuid.UUID
	if err := tx.QueryRow(ctx, `
		DELETE FROM sender_provider_bindings AS binding
		USING sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.team_id = $2
		  AND asset.channel = 'sms'
		RETURNING binding.sender_asset_id
	`, id, teamID).Scan(&assetID); err != nil {
		return SenderID{}, fmt.Errorf("delete sender provider binding: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM sender_assets AS asset
		WHERE asset.id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM sender_provider_bindings AS binding
			WHERE binding.sender_asset_id = asset.id
		  )
	`, assetID); err != nil {
		return SenderID{}, fmt.Errorf("delete unbound sender asset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderID{}, fmt.Errorf("commit sender id deletion: %w", err)
	}
	return sender, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getSenderID(ctx context.Context, db queryRower, id, teamID uuid.UUID, lock bool) (SenderID, error) {
	query := `
		SELECT ` + senderIDProjection + `
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $2
		 AND grant_record.channel = 'sms'
		 AND grant_record.status = 'active'
		JOIN teams AS team ON team.id = grant_record.team_id
		WHERE binding.id = $1
		  AND asset.channel = 'sms'
		  AND team.status = 'active'`
	if lock {
		query += " FOR UPDATE OF binding, asset"
	}
	sender, err := scanSenderID(db.QueryRow(ctx, query, id, teamID))
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id: %w", err)
	}
	return sender, nil
}

func scanSenderID(scanner rowScanner) (SenderID, error) {
	var id, teamID uuid.UUID
	var provider, rejectionReason *string
	var approvedAt, rejectedAt, suspendedAt, createdAt, updatedAt pgtype.Timestamptz
	var createdBy *uuid.UUID
	var name, countryCode, purpose, status string
	if err := scanner.Scan(
		&id,
		&teamID,
		&name,
		&countryCode,
		&purpose,
		&status,
		&provider,
		&rejectionReason,
		&approvedAt,
		&rejectedAt,
		&suspendedAt,
		&createdBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return SenderID{}, err
	}
	var createdByString *string
	if createdBy != nil {
		value := createdBy.String()
		createdByString = &value
	}
	return SenderID{
		ID:              id.String(),
		TeamID:          teamID.String(),
		Name:            name,
		CountryCode:     countryCode,
		Purpose:         purpose,
		Status:          status,
		Provider:        provider,
		RejectionReason: rejectionReason,
		ApprovedAt:      timestamptzPtr(approvedAt),
		RejectedAt:      timestamptzPtr(rejectedAt),
		SuspendedAt:     timestamptzPtr(suspendedAt),
		CreatedBy:       createdByString,
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}, nil
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
''')

Path("internal/modules/domain/repository.go").write_text(r'''package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSenderDomainAlreadyExists = errors.New("sender domain already exists")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type ReconciliationClaim struct {
	Domain  SenderDomain
	Attempt int32
}

type rowScanner interface {
	Scan(dest ...any) error
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const senderDomainProjection = `
	binding.id,
	asset.team_id,
	asset.normalized_identity,
	COALESCE(CASE binding.provider WHEN 'ses' THEN 'aws_ses' ELSE binding.provider END, ''),
	COALESCE(binding.region, ''),
	CASE binding.status
		WHEN 'active' THEN 'verified'
		WHEN 'rejected' THEN 'failed'
		ELSE binding.status
	END,
	COALESCE(binding.verification_data -> 'records', '[]'::jsonb),
	COALESCE(binding.rejection_reason, binding.last_error),
	binding.health_status,
	binding.consecutive_health_failures,
	binding.last_checked_at,
	binding.last_health_checked_at,
	binding.last_health_failure_at,
	binding.verified_at,
	binding.disabled_at,
	asset.created_by,
	binding.created_at,
	binding.updated_at,
	binding.attempts`

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	domain string,
	provider string,
	providerRegion string,
	records []VerificationRecord,
	createdBy uuid.UUID,
) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	normalizedDomain := strings.ToLower(strings.TrimSpace(domain))
	normalizedProvider := normalizeProvider(provider)
	normalizedRegion := strings.ToLower(strings.TrimSpace(providerRegion))

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var teamActive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE id = $1 AND status = 'active')`, teamID).Scan(&teamActive); err != nil {
		return SenderDomain{}, fmt.Errorf("verify sender domain team: %w", err)
	}
	if !teamActive {
		return SenderDomain{}, pgx.ErrNoRows
	}

	var assetID uuid.UUID
	var ownerType string
	var ownerTeamID *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, owner_type, team_id
		FROM sender_assets
		WHERE channel = 'email'
		  AND normalized_identity = $1
		FOR UPDATE
	`, normalizedDomain).Scan(&assetID, &ownerType, &ownerTeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO sender_assets (
				owner_type, team_id, channel, identity, normalized_identity,
				purpose, status, health_status, created_by
			) VALUES (
				'team', $1, 'email', $2, $2,
				'Email sending domain', 'pending', 'unknown', $3
			)
			RETURNING id
		`, teamID, normalizedDomain, createdBy).Scan(&assetID)
		if err != nil {
			if isUniqueViolation(err) {
				return SenderDomain{}, ErrSenderDomainAlreadyExists
			}
			return SenderDomain{}, fmt.Errorf("create email sender asset: %w", err)
		}
	} else if err != nil {
		return SenderDomain{}, fmt.Errorf("lock email sender asset: %w", err)
	} else if ownerType != "team" || ownerTeamID == nil || *ownerTeamID != teamID {
		return SenderDomain{}, ErrSenderDomainAlreadyExists
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sender_asset_grants (
			team_id, sender_asset_id, channel, status, granted_by
		) VALUES ($1, $2, 'email', 'active', $3)
		ON CONFLICT (team_id, sender_asset_id)
		DO UPDATE SET status = 'active', revoked_at = NULL, updated_at = now()
	`, teamID, assetID, createdBy); err != nil {
		return SenderDomain{}, fmt.Errorf("grant email sender asset: %w", err)
	}

	var bindingID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sender_provider_bindings (
			sender_asset_id, provider, region, status, provider_status,
			verified, health_status, verification_data, next_check_at
		) VALUES (
			$1, $2, $3, 'pending', 'pending', false, 'unknown',
			jsonb_build_object('records', $4::jsonb), now() + interval '1 minute'
		)
		RETURNING id
	`, assetID, normalizedProvider, normalizedRegion, recordsJSON).Scan(&bindingID)
	if err != nil {
		if isUniqueViolation(err) {
			return SenderDomain{}, ErrSenderDomainAlreadyExists
		}
		return SenderDomain{}, fmt.Errorf("create email sender binding: %w", err)
	}

	result, _, err := getSenderDomain(ctx, tx, bindingID, teamID, false)
	if err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain creation: %w", err)
	}
	return result, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderDomain, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sender domain repository is not configured")
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+senderDomainProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $1
		 AND grant_record.channel = 'email'
		 AND grant_record.status = 'active'
		WHERE asset.channel = 'email'
		ORDER BY binding.created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list sender domains: %w", err)
	}
	defer rows.Close()

	domains := make([]SenderDomain, 0)
	for rows.Next() {
		domain, _, err := scanSenderDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sender domain: %w", err)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sender domains: %w", err)
	}
	return domains, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	domain, _, err := getSenderDomain(ctx, r.db, id, teamID, false)
	return domain, err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	domain, _, err := getSenderDomain(ctx, tx, id, teamID, true)
	if err != nil {
		return SenderDomain{}, err
	}
	assetID, err := deleteDomainBinding(ctx, tx, id, teamID, false)
	if err != nil {
		return SenderDomain{}, err
	}
	if err := deleteUnboundAsset(ctx, tx, assetID); err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain deletion: %w", err)
	}
	return domain, nil
}

func (r *Repository) UpdateVerification(ctx context.Context, id, teamID uuid.UUID, status string, records []VerificationRecord, failureReason *string) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	if err := r.updateVerification(ctx, id, teamID, "", status, recordsJSON, failureReason, time.Time{}); err != nil {
		return SenderDomain{}, fmt.Errorf("update sender domain verification: %w", err)
	}
	return r.Get(ctx, id, teamID)
}

func (r *Repository) ClaimPendingReconciliations(ctx context.Context, workerID string, limit int32, staleBefore time.Time) ([]ReconciliationClaim, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sender domain repository is not configured")
	}
	rows, err := r.db.Query(ctx, `
		WITH candidates AS (
			SELECT binding.id
			FROM sender_provider_bindings AS binding
			JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
			WHERE asset.channel = 'email'
			  AND asset.owner_type = 'team'
			  AND binding.status IN ('pending', 'active')
			  AND binding.disabled_at IS NULL
			  AND binding.next_check_at <= now()
			  AND (binding.reconcile_locked_at IS NULL OR binding.reconcile_locked_at < $3)
			ORDER BY binding.next_check_at, binding.created_at
			FOR UPDATE OF binding SKIP LOCKED
			LIMIT $2
		), updated AS (
			UPDATE sender_provider_bindings AS binding
			SET reconcile_locked_at = now(),
				reconcile_locked_by = $1,
				attempts = binding.attempts + 1,
				updated_at = now()
			FROM candidates
			WHERE binding.id = candidates.id
			RETURNING binding.*
		)
		SELECT `+senderDomainProjection+`
		FROM updated AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		ORDER BY binding.next_check_at, binding.created_at
	`, strings.TrimSpace(workerID), limit, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("claim sender domain reconciliations: %w", err)
	}
	defer rows.Close()

	claims := make([]ReconciliationClaim, 0)
	for rows.Next() {
		domain, attempt, err := scanSenderDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sender domain reconciliation claim: %w", err)
		}
		claims = append(claims, ReconciliationClaim{Domain: domain, Attempt: attempt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sender domain reconciliation claims: %w", err)
	}
	return claims, nil
}

func (r *Repository) CompleteReconciliation(ctx context.Context, id uuid.UUID, workerID, status string, records []VerificationRecord, nextCheckAt time.Time) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	if err := r.updateVerification(ctx, id, uuid.Nil, workerID, status, recordsJSON, nil, nextCheckAt); err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain reconciliation: %w", err)
	}
	return r.getByID(ctx, id)
}

func (r *Repository) RecordReconciliationFailure(ctx context.Context, id uuid.UUID, workerID string, cause error, nextCheckAt time.Time) (SenderDomain, error) {
	reason := "sender domain reconciliation failed"
	if cause != nil {
		reason = cause.Error()
	}
	result, err := r.db.Exec(ctx, `
		UPDATE sender_provider_bindings
		SET last_error = $3,
			last_checked_at = now(),
			next_check_at = $4,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND reconcile_locked_by = $2
	`, id, strings.TrimSpace(workerID), reason, nextCheckAt)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain reconciliation failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return SenderDomain{}, pgx.ErrNoRows
	}
	return r.getByID(ctx, id)
}

func (r *Repository) CompleteHealthCheck(ctx context.Context, id uuid.UUID, workerID string, nextCheckAt time.Time) (SenderDomain, error) {
	result, err := r.db.Exec(ctx, `
		UPDATE sender_provider_bindings
		SET health_status = 'healthy',
			consecutive_health_failures = 0,
			last_error = NULL,
			last_checked_at = now(),
			last_health_checked_at = now(),
			next_check_at = $3,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND status = 'active'
		  AND reconcile_locked_by = $2
	`, id, strings.TrimSpace(workerID), nextCheckAt)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain health check: %w", err)
	}
	if result.RowsAffected() != 1 {
		return SenderDomain{}, pgx.ErrNoRows
	}
	return r.getByID(ctx, id)
}

func (r *Repository) RecordHealthFailure(ctx context.Context, id uuid.UUID, workerID string, cause error, failureThreshold int32, nextCheckAt time.Time) (SenderDomain, error) {
	reason := "sender domain health check failed"
	if cause != nil {
		reason = cause.Error()
	}
	result, err := r.db.Exec(ctx, `
		UPDATE sender_provider_bindings
		SET health_status = CASE
				WHEN consecutive_health_failures + 1 >= $4 THEN 'degraded'
				ELSE health_status
			END,
			consecutive_health_failures = consecutive_health_failures + 1,
			last_error = $3,
			last_checked_at = now(),
			last_health_checked_at = now(),
			last_health_failure_at = now(),
			next_check_at = $5,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND status = 'active'
		  AND reconcile_locked_by = $2
	`, id, strings.TrimSpace(workerID), reason, failureThreshold, nextCheckAt)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain health failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return SenderDomain{}, pgx.ErrNoRows
	}
	return r.getByID(ctx, id)
}

func (r *Repository) UpdateManualHealthCheck(ctx context.Context, id, teamID uuid.UUID, records []VerificationRecord, failureReason *string) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE sender_provider_bindings AS binding
		SET verification_data = jsonb_set(binding.verification_data, '{records}', $3::jsonb, true),
			health_status = CASE
				WHEN $4::text IS NULL THEN 'healthy'
				WHEN binding.consecutive_health_failures + 1 >= $5 THEN 'degraded'
				ELSE binding.health_status
			END,
			consecutive_health_failures = CASE
				WHEN $4::text IS NULL THEN 0
				ELSE binding.consecutive_health_failures + 1
			END,
			last_error = $4,
			last_checked_at = now(),
			last_health_checked_at = now(),
			last_health_failure_at = CASE WHEN $4::text IS NULL THEN binding.last_health_failure_at ELSE now() END,
			updated_at = now()
		FROM sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.team_id = $2
		  AND asset.channel = 'email'
		  AND binding.status = 'active'
	`, id, teamID, recordsJSON, failureReason, DefaultHealthFailureThreshold)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update manual sender domain health check: %w", err)
	}
	if result.RowsAffected() != 1 {
		return SenderDomain{}, errors.New("verified sender domain not found")
	}
	return r.Get(ctx, id, teamID)
}

func (r *Repository) Disable(ctx context.Context, id, teamID uuid.UUID) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain disable: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var assetID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE sender_provider_bindings AS binding
		SET status = 'disabled',
			disabled_at = COALESCE(binding.disabled_at, now()),
			next_check_at = now(),
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		FROM sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.team_id = $2
		  AND asset.channel = 'email'
		RETURNING binding.sender_asset_id
	`, id, teamID).Scan(&assetID)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("disable sender domain: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sender_assets AS asset
		SET status = 'disabled', updated_at = now()
		WHERE asset.id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM sender_provider_bindings AS binding
			WHERE binding.sender_asset_id = asset.id
			  AND binding.status <> 'disabled'
		  )
	`, assetID); err != nil {
		return SenderDomain{}, fmt.Errorf("disable sender asset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain disable: %w", err)
	}
	return r.Get(ctx, id, teamID)
}

func (r *Repository) PurgeIfUnreferenced(ctx context.Context, id, teamID uuid.UUID) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("sender domain repository is not configured")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin sender domain purge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	assetID, err := deleteDomainBinding(ctx, tx, id, teamID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if err := deleteUnboundAsset(ctx, tx, assetID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit sender domain purge: %w", err)
	}
	return true, nil
}

func (r *Repository) getByID(ctx context.Context, id uuid.UUID) (SenderDomain, error) {
	domain, _, err := scanSenderDomain(r.db.QueryRow(ctx, `
		SELECT `+senderDomainProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		WHERE binding.id = $1
		  AND asset.channel = 'email'
	`, id))
	if err != nil {
		return SenderDomain{}, fmt.Errorf("get sender domain: %w", err)
	}
	return domain, nil
}

func (r *Repository) updateVerification(ctx context.Context, id, teamID uuid.UUID, workerID, status string, recordsJSON []byte, failureReason *string, nextCheckAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin sender domain verification update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	bindingStatus := status
	if status == StatusVerified {
		bindingStatus = "active"
	}
	var assetID uuid.UUID
	query := `
		UPDATE sender_provider_bindings AS binding
		SET status = $3,
			provider_status = $4,
			verified = $4 = 'verified',
			verification_data = jsonb_set(binding.verification_data, '{records}', $5::jsonb, true),
			rejection_reason = CASE WHEN $4 = 'failed' THEN $6 ELSE NULL END,
			last_error = $6,
			last_checked_at = now(),
			health_status = CASE WHEN $4 = 'verified' THEN 'healthy' ELSE binding.health_status END,
			consecutive_health_failures = CASE WHEN $4 = 'verified' THEN 0 ELSE binding.consecutive_health_failures END,
			last_health_checked_at = CASE WHEN $4 = 'verified' THEN now() ELSE binding.last_health_checked_at END,
			verified_at = CASE WHEN $4 = 'verified' THEN COALESCE(binding.verified_at, now()) ELSE binding.verified_at END,
			next_check_at = CASE WHEN $7::timestamptz IS NULL THEN binding.next_check_at ELSE $7 END,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		FROM sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.channel = 'email'`
	args := []any{id, teamID, bindingStatus, status, recordsJSON, failureReason, nullableTime(nextCheckAt)}
	if workerID != "" {
		query += " AND binding.reconcile_locked_by = $2"
		args[1] = strings.TrimSpace(workerID)
	} else {
		query += " AND asset.team_id = $2"
	}
	query += " RETURNING binding.sender_asset_id"
	if err := tx.QueryRow(ctx, query, args...).Scan(&assetID); err != nil {
		return err
	}
	assetStatus := bindingStatus
	if status == StatusFailed {
		assetStatus = "failed"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sender_assets
		SET status = $2,
			health_status = CASE WHEN $3 = 'verified' THEN 'healthy' ELSE health_status END,
			updated_at = now()
		WHERE id = $1
	`, assetID, assetStatus, status); err != nil {
		return fmt.Errorf("update email sender asset state: %w", err)
	}
	return tx.Commit(ctx)
}

func getSenderDomain(ctx context.Context, db queryRower, id, teamID uuid.UUID, lock bool) (SenderDomain, int32, error) {
	query := `
		SELECT ` + senderDomainProjection + `
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $2
		 AND grant_record.channel = 'email'
		 AND grant_record.status = 'active'
		WHERE binding.id = $1
		  AND asset.channel = 'email'`
	if lock {
		query += " FOR UPDATE OF binding, asset"
	}
	domain, attempt, err := scanSenderDomain(db.QueryRow(ctx, query, id, teamID))
	if err != nil {
		return SenderDomain{}, 0, fmt.Errorf("get sender domain: %w", err)
	}
	return domain, attempt, nil
}

func scanSenderDomain(scanner rowScanner) (SenderDomain, int32, error) {
	var id, teamID uuid.UUID
	var domain, provider, region, status, healthStatus string
	var recordsJSON []byte
	var failureReason *string
	var lastCheckedAt, lastHealthCheckedAt, lastHealthFailureAt pgtype.Timestamptz
	var verifiedAt, disabledAt, createdAt, updatedAt pgtype.Timestamptz
	var createdBy *uuid.UUID
	var failures, attempts int32
	if err := scanner.Scan(
		&id,
		&teamID,
		&domain,
		&provider,
		&region,
		&status,
		&recordsJSON,
		&failureReason,
		&healthStatus,
		&failures,
		&lastCheckedAt,
		&lastHealthCheckedAt,
		&lastHealthFailureAt,
		&verifiedAt,
		&disabledAt,
		&createdBy,
		&createdAt,
		&updatedAt,
		&attempts,
	); err != nil {
		return SenderDomain{}, 0, err
	}
	var records []VerificationRecord
	if err := json.Unmarshal(recordsJSON, &records); err != nil {
		records = []VerificationRecord{}
	}
	var createdByString *string
	if createdBy != nil {
		value := createdBy.String()
		createdByString = &value
	}
	return SenderDomain{
		ID:                        id.String(),
		TeamID:                    teamID.String(),
		Domain:                    domain,
		Provider:                  provider,
		ProviderRegion:            region,
		Status:                    status,
		VerificationRecords:       records,
		FailureReason:             failureReason,
		HealthStatus:              healthStatus,
		ConsecutiveHealthFailures: failures,
		LastCheckedAt:             timestamptzPtr(lastCheckedAt),
		LastHealthCheckedAt:       timestamptzPtr(lastHealthCheckedAt),
		LastHealthFailureAt:       timestamptzPtr(lastHealthFailureAt),
		VerifiedAt:                timestamptzPtr(verifiedAt),
		DisabledAt:                timestamptzPtr(disabledAt),
		CreatedBy:                 createdByString,
		CreatedAt:                 createdAt.Time,
		UpdatedAt:                 updatedAt.Time,
	}, attempts, nil
}

func deleteDomainBinding(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID, requireDisabled bool) (uuid.UUID, error) {
	query := `
		DELETE FROM sender_provider_bindings AS binding
		USING sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.team_id = $2
		  AND asset.channel = 'email'`
	if requireDisabled {
		query += `
		  AND binding.status = 'disabled'
		  AND NOT EXISTS (
			SELECT 1 FROM email_messages AS message
			WHERE message.sender_provider_binding_id = binding.id
		  )`
	}
	query += " RETURNING binding.sender_asset_id"
	var assetID uuid.UUID
	if err := tx.QueryRow(ctx, query, id, teamID).Scan(&assetID); err != nil {
		return uuid.Nil, err
	}
	return assetID, nil
}

func deleteUnboundAsset(ctx context.Context, tx pgx.Tx, assetID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM sender_assets AS asset
		WHERE asset.id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM sender_provider_bindings AS binding
			WHERE binding.sender_asset_id = asset.id
		  )
	`, assetID); err != nil {
		return fmt.Errorf("delete unbound sender asset: %w", err)
	}
	return nil
}

func normalizeProvider(provider string) string {
	value := strings.ToLower(strings.TrimSpace(provider))
	if value == "aws_ses" {
		return "ses"
	}
	return value
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	result := value
	return &result
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
''')

sender_delivery = Path("internal/delivery/senderid/repository.go")
source = sender_delivery.read_text()
source = source.replace("FROM sender_ids", "FROM sender_provider_bindings AS binding\n\t\t\tJOIN sender_assets AS asset ON asset.id = binding.sender_asset_id")
source = source.replace("UPDATE sender_ids AS sender", "UPDATE sender_provider_bindings AS binding")
source = source.replace("sender.", "binding.")
source = source.replace("WHERE binding.id = candidates.id", "WHERE binding.id = candidates.id")
source = source.replace("binding.name", "asset.identity")
source = source.replace("binding.country_code", "COALESCE(binding.country_code::text, '')")
source = source.replace("binding.provider_submitted_at", "binding.submitted_at")
source = source.replace("registration_locked_at", "reconcile_locked_at")
source = source.replace("registration_locked_by", "reconcile_locked_by")
source = source.replace("provider_attempts", "attempts")
source = source.replace("provider_last_checked_at", "last_checked_at")
source = source.replace("next_status_check_at", "next_check_at")
source = source.replace("provider_error", "last_error")
source = source.replace("provider_submitted_at", "submitted_at")
source = source.replace("approved_at", "verified_at")
source = source.replace("UPDATE sender_ids", "UPDATE sender_provider_bindings")
source = source.replace("status = $3,", "status = CASE $3 WHEN 'approved' THEN 'active' WHEN 'inactive' THEN 'disabled' ELSE $3 END,\n\t\t\tverified = $3 = 'approved',", 1)
source = source.replace("WHEN $3 = 'approved' THEN COALESCE(verified_at, now())", "WHEN $3 = 'approved' THEN COALESCE(verified_at, now())")
sender_delivery.write_text(source)

for path in [
    "internal/database/queries/sender_ids.sql",
    "internal/database/queries/sender_domains.sql",
    "internal/database/sqlc/sender_ids.sql.go",
    "internal/database/sqlc/sender_domains.sql.go",
]:
    file_path = Path(path)
    if file_path.exists():
        file_path.unlink()
