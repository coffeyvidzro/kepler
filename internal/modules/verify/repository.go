package verify

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var (
	ErrNotFound     = errors.New("verify resource not found")
	ErrDuplicateKey = errors.New("verification service key already exists")
)

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
	tx      pgx.Tx
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (repository *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: repository.db, queries: repository.queries.WithTx(tx), tx: tx}
}

func (repository *Repository) CreateService(ctx context.Context, teamID uuid.UUID, value validatedService) (VerificationService, error) {
	row, err := repository.queries.CreateVerificationService(ctx, dbsqlc.CreateVerificationServiceParams{
		Key: value.Key, Name: value.Name, DefaultChannel: value.DefaultChannel,
		CodeLength: value.CodeLength, TtlSeconds: value.TTLSeconds, MaxAttempts: value.MaxAttempts,
		ResendCooldownSeconds: value.ResendCooldownSeconds, MaxResends: value.MaxResends,
		Enabled: value.Enabled, Metadata: value.Metadata, TeamID: teamID,
	})
	if isUniqueViolation(err) {
		return VerificationService{}, ErrDuplicateKey
	}
	if err != nil {
		return VerificationService{}, fmt.Errorf("create verification service: %w", err)
	}
	return serviceFromSQLC(row), nil
}

func (repository *Repository) GetService(ctx context.Context, id, teamID uuid.UUID) (VerificationService, error) {
	row, err := repository.queries.GetVerificationService(ctx, dbsqlc.GetVerificationServiceParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationService{}, ErrNotFound
	}
	if err != nil {
		return VerificationService{}, fmt.Errorf("get verification service: %w", err)
	}
	return serviceFromSQLC(row), nil
}

func (repository *Repository) GetServiceByKey(ctx context.Context, teamID uuid.UUID, key string) (VerificationService, error) {
	row, err := repository.queries.GetVerificationServiceByKey(ctx, dbsqlc.GetVerificationServiceByKeyParams{TeamID: teamID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationService{}, ErrNotFound
	}
	if err != nil {
		return VerificationService{}, fmt.Errorf("get verification service by key: %w", err)
	}
	return serviceFromSQLC(row), nil
}

func (repository *Repository) ListServices(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]VerificationService, error) {
	rows, err := repository.queries.ListVerificationServices(ctx, dbsqlc.ListVerificationServicesParams{
		TeamID: teamID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list verification services: %w", err)
	}
	result := make([]VerificationService, 0, len(rows))
	for _, row := range rows {
		result = append(result, serviceFromSQLC(row))
	}
	return result, nil
}

func (repository *Repository) UpdateService(ctx context.Context, id, teamID uuid.UUID, value validatedService) (VerificationService, error) {
	row, err := repository.queries.UpdateVerificationService(ctx, dbsqlc.UpdateVerificationServiceParams{
		Name: value.Name, DefaultChannel: value.DefaultChannel, CodeLength: value.CodeLength,
		TtlSeconds: value.TTLSeconds, MaxAttempts: value.MaxAttempts,
		ResendCooldownSeconds: value.ResendCooldownSeconds, MaxResends: value.MaxResends,
		Metadata: value.Metadata, ID: id, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationService{}, ErrNotFound
	}
	if err != nil {
		return VerificationService{}, fmt.Errorf("update verification service: %w", err)
	}
	if row.Enabled != value.Enabled {
		row, err = repository.queries.SetVerificationServiceEnabled(ctx, dbsqlc.SetVerificationServiceEnabledParams{
			Enabled: value.Enabled, ID: id, TeamID: teamID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return VerificationService{}, ErrNotFound
		}
		if err != nil {
			return VerificationService{}, fmt.Errorf("set verification service enabled: %w", err)
		}
	}
	return serviceFromSQLC(row), nil
}

func (repository *Repository) CreateVerification(ctx context.Context, teamID, serviceID uuid.UUID, value validatedVerification, expiresAt pgtype.Timestamptz) (Verification, error) {
	row, err := repository.queries.CreateVerification(ctx, dbsqlc.CreateVerificationParams{
		Channel: value.Channel, Recipient: value.Recipient, RecipientNormalized: value.RecipientNormalized,
		Locale: value.Locale, Metadata: value.Metadata, ExpiresAt: expiresAt, ServiceID: serviceID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("create verification: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) CreateChallenge(ctx context.Context, teamID, verificationID uuid.UUID, sequence int32, codeHMAC []byte, channel string, expiresAt pgtype.Timestamptz) (dbsqlc.VerificationChallenge, error) {
	row, err := repository.queries.CreateVerificationChallenge(ctx, dbsqlc.CreateVerificationChallengeParams{
		Sequence: sequence, CodeHmac: codeHMAC, Channel: channel, ExpiresAt: expiresAt,
		VerificationID: verificationID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.VerificationChallenge{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.VerificationChallenge{}, fmt.Errorf("create verification challenge: %w", err)
	}
	return row, nil
}

func (repository *Repository) GetVerification(ctx context.Context, id, teamID uuid.UUID) (Verification, error) {
	row, err := repository.queries.GetVerification(ctx, dbsqlc.GetVerificationParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("get verification: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) GetVerificationForUpdate(ctx context.Context, id, teamID uuid.UUID) (dbsqlc.Verification, error) {
	row, err := repository.queries.GetVerificationForUpdate(ctx, dbsqlc.GetVerificationForUpdateParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.Verification{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.Verification{}, fmt.Errorf("lock verification: %w", err)
	}
	return row, nil
}

func (repository *Repository) ListVerifications(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Verification, error) {
	rows, err := repository.queries.ListVerifications(ctx, dbsqlc.ListVerificationsParams{
		TeamID: teamID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list verifications: %w", err)
	}
	result := make([]Verification, 0, len(rows))
	for _, row := range rows {
		result = append(result, verificationFromSQLC(row))
	}
	return result, nil
}

func (repository *Repository) GetActiveChallengeForUpdate(ctx context.Context, verificationID, teamID uuid.UUID) (dbsqlc.VerificationChallenge, error) {
	row, err := repository.queries.GetActiveVerificationChallengeForUpdate(ctx, dbsqlc.GetActiveVerificationChallengeForUpdateParams{
		VerificationID: verificationID, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.VerificationChallenge{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.VerificationChallenge{}, fmt.Errorf("lock active verification challenge: %w", err)
	}
	return row, nil
}

func (repository *Repository) IncrementAttempt(ctx context.Context, id, teamID uuid.UUID) (dbsqlc.Verification, error) {
	row, err := repository.queries.IncrementVerificationAttemptCount(ctx, dbsqlc.IncrementVerificationAttemptCountParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.Verification{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.Verification{}, fmt.Errorf("increment verification attempt: %w", err)
	}
	return row, nil
}

func (repository *Repository) RecordAttempt(ctx context.Context, verificationID, challengeID, teamID uuid.UUID, result string, req CheckRequest) error {
	_, err := repository.queries.CreateVerificationAttempt(ctx, dbsqlc.CreateVerificationAttemptParams{
		Result: result, IpAddressHash: req.IPHash, UserAgent: req.UserAgent, Metadata: req.Metadata,
		ChallengeID: challengeID, VerificationID: verificationID, TeamID: teamID,
	})
	if err != nil {
		return fmt.Errorf("create verification attempt: %w", err)
	}
	return nil
}

func (repository *Repository) Approve(ctx context.Context, id, teamID uuid.UUID) (Verification, error) {
	row, err := repository.queries.ApproveVerification(ctx, dbsqlc.ApproveVerificationParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("approve verification: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) MarkExpired(ctx context.Context, id, teamID uuid.UUID) (Verification, error) {
	row, err := repository.queries.ExpireVerification(ctx, dbsqlc.ExpireVerificationParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("expire verification: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) MarkMaxAttempts(ctx context.Context, id, teamID uuid.UUID) (Verification, error) {
	row, err := repository.queries.MarkVerificationMaxAttemptsReached(ctx, dbsqlc.MarkVerificationMaxAttemptsReachedParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("mark verification max attempts: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) Cancel(ctx context.Context, id, teamID uuid.UUID) (Verification, error) {
	row, err := repository.queries.CancelVerification(ctx, dbsqlc.CancelVerificationParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Verification{}, ErrNotFound
	}
	if err != nil {
		return Verification{}, fmt.Errorf("cancel verification: %w", err)
	}
	return verificationFromSQLC(row), nil
}

func (repository *Repository) IncrementResend(ctx context.Context, id, teamID uuid.UUID, expiresAt pgtype.Timestamptz) (dbsqlc.Verification, error) {
	row, err := repository.queries.IncrementVerificationResendCount(ctx, dbsqlc.IncrementVerificationResendCountParams{
		ExpiresAt: expiresAt, ID: id, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.Verification{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.Verification{}, fmt.Errorf("increment verification resend: %w", err)
	}
	return row, nil
}

func (repository *Repository) SupersedeChallenges(ctx context.Context, verificationID, teamID uuid.UUID) error {
	_, err := repository.queries.SupersedeActiveVerificationChallenges(ctx, dbsqlc.SupersedeActiveVerificationChallengesParams{
		VerificationID: verificationID, TeamID: teamID,
	})
	if err != nil {
		return fmt.Errorf("supersede verification challenges: %w", err)
	}
	return nil
}

func (repository *Repository) MarkChallengeExpired(ctx context.Context, id, teamID uuid.UUID) error {
	_, err := repository.queries.MarkVerificationChallengeExpired(ctx, dbsqlc.MarkVerificationChallengeExpiredParams{ID: id, TeamID: teamID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("expire verification challenge: %w", err)
	}
	return nil
}

func serviceFromSQLC(row dbsqlc.VerificationService) VerificationService {
	return VerificationService{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Key: row.Key, Name: row.Name,
		DefaultChannel: row.DefaultChannel, CodeLength: row.CodeLength, TTLSeconds: row.TtlSeconds,
		MaxAttempts: row.MaxAttempts, ResendCooldownSeconds: row.ResendCooldownSeconds,
		MaxResends: row.MaxResends, Enabled: row.Enabled, Metadata: ensureMetadata(row.Metadata),
		CreatedAt: pgconv.TimestamptzToTime(row.CreatedAt), UpdatedAt: pgconv.TimestamptzToTime(row.UpdatedAt),
	}
}

func verificationFromSQLC(row dbsqlc.Verification) Verification {
	return Verification{
		ID: row.ID.String(), TeamID: row.TeamID.String(), ServiceID: row.ServiceID.String(),
		Channel: row.Channel, Recipient: row.Recipient, Status: row.Status, Locale: row.Locale,
		Metadata: ensureMetadata(row.Metadata), AttemptCount: row.AttemptCount, ResendCount: row.ResendCount,
		ExpiresAt: pgconv.TimestamptzToTime(row.ExpiresAt), ApprovedAt: pgconv.TimestamptzToTimePtr(row.ApprovedAt),
		ExpiredAt: pgconv.TimestamptzToTimePtr(row.ExpiredAt), CanceledAt: pgconv.TimestamptzToTimePtr(row.CanceledAt),
		FailedAt: pgconv.TimestamptzToTimePtr(row.FailedAt), CreatedAt: pgconv.TimestamptzToTime(row.CreatedAt),
		UpdatedAt: pgconv.TimestamptzToTime(row.UpdatedAt),
	}
}

func ensureMetadata(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
