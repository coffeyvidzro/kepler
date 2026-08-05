package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

const lockVerificationTokenIdentifierSQL = `
SELECT pg_advisory_xact_lock(
    hashtextextended('verification-token:' || $1, 0)
)
`

type UserRecord struct {
	ID                uuid.UUID
	Email             string
	EmailVerified     bool
	Name              string
	PasswordHash      *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CredentialVersion int64
	SecurityUpdatedAt time.Time
}

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) CreateUser(
	ctx context.Context,
	name string,
	email string,
	passwordHash string,
) (UserRecord, error) {
	row, err := r.queries.CreateUser(
		ctx,
		dbsqlc.CreateUserParams{Name: name, Email: email, PasswordHash: &passwordHash},
	)
	if err != nil {
		return UserRecord{}, fmt.Errorf("create identity user: %w", err)
	}
	return userRecordFromSQLC(row), nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	row, err := r.queries.GetUserByEmail(ctx, dbsqlc.GetUserByEmailParams{Email: email})
	if err != nil {
		return UserRecord{}, fmt.Errorf("get identity user by email: %w", err)
	}
	return userRecordFromSQLC(row), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (UserRecord, error) {
	row, err := r.queries.GetUserByID(ctx, dbsqlc.GetUserByIDParams{ID: id})
	if err != nil {
		return UserRecord{}, fmt.Errorf("get identity user by id: %w", err)
	}
	return userRecordFromSQLC(row), nil
}

func (r *Repository) GetPrincipalByUserID(
	ctx context.Context,
	id string,
) (authnz.Principal, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return authnz.Principal{}, fmt.Errorf("parse principal user id: %w", err)
	}

	user, err := r.GetUserByID(ctx, parsedID)
	if err != nil {
		return authnz.Principal{}, err
	}

	return authnz.Principal{
		UserID:            user.ID,
		Email:             user.Email,
		Name:              user.Name,
		EmailVerified:     user.EmailVerified,
		CredentialVersion: user.CredentialVersion,
	}, nil
}

func (r *Repository) CreateVerificationToken(
	ctx context.Context,
	identifier string,
	tokenHash string,
	expiresAt time.Time,
) error {
	tx, queries, err := r.beginVerificationTokenTransaction(ctx, identifier)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := queries.DeleteVerificationTokensByIdentifier(
		ctx,
		dbsqlc.DeleteVerificationTokensByIdentifierParams{Identifier: identifier},
	); err != nil {
		return fmt.Errorf("delete superseded verification tokens: %w", err)
	}
	if _, err := queries.CreateVerificationToken(ctx, dbsqlc.CreateVerificationTokenParams{
		Identifier: identifier,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit verification token replacement: %w", err)
	}
	return nil
}

func (r *Repository) VerifyEmailWithToken(ctx context.Context, email string, identifier string, tokenHash string) (UserRecord, error) {
	row, err := r.queries.VerifyEmailWithToken(ctx, dbsqlc.VerifyEmailWithTokenParams{Email: email, Identifier: identifier, TokenHash: tokenHash})
	if err != nil {
		return UserRecord{}, fmt.Errorf("verify email with token: %w", err)
	}
	return userRecordFromSQLC(row), nil
}

func (r *Repository) ResetPasswordWithToken(
	ctx context.Context,
	email string,
	identifier string,
	tokenHash string,
	passwordHash string,
) (UserRecord, error) {
	tx, queries, err := r.beginVerificationTokenTransaction(ctx, identifier)
	if err != nil {
		return UserRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := queries.ResetPasswordWithToken(ctx, dbsqlc.ResetPasswordWithTokenParams{
		Email: email, Identifier: identifier, TokenHash: tokenHash, PasswordHash: &passwordHash,
	})
	if err != nil {
		return UserRecord{}, fmt.Errorf("reset password with token: %w", err)
	}
	if err := queries.DeleteVerificationTokensByIdentifier(
		ctx,
		dbsqlc.DeleteVerificationTokensByIdentifierParams{Identifier: identifier},
	); err != nil {
		return UserRecord{}, fmt.Errorf("delete remaining password reset tokens: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return UserRecord{}, fmt.Errorf("commit password reset: %w", err)
	}
	return userRecordFromValues(row.ID, row.Email, row.EmailVerified, row.Name, row.PasswordHash, row.CreatedAt.Time, row.UpdatedAt.Time, row.CredentialVersion, row.SecurityUpdatedAt.Time), nil
}

func (r *Repository) beginVerificationTokenTransaction(
	ctx context.Context,
	identifier string,
) (pgx.Tx, *dbsqlc.Queries, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin verification token transaction: %w", err)
	}
	if _, err := tx.Exec(ctx, lockVerificationTokenIdentifierSQL, identifier); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("lock verification token identifier: %w", err)
	}
	return tx, r.queries.WithTx(tx), nil
}

func userRecordFromSQLC(row dbsqlc.User) UserRecord {
	return userRecordFromValues(row.ID, row.Email, row.EmailVerified, row.Name, row.PasswordHash, row.CreatedAt.Time, row.UpdatedAt.Time, row.CredentialVersion, row.SecurityUpdatedAt.Time)
}

func userRecordFromValues(id uuid.UUID, email string, verified bool, name string, passwordHash *string, createdAt time.Time, updatedAt time.Time, credentialVersion int64, securityUpdatedAt time.Time) UserRecord {
	return UserRecord{
		ID: id, Email: email, EmailVerified: verified, Name: name, PasswordHash: passwordHash,
		CreatedAt: createdAt, UpdatedAt: updatedAt, CredentialVersion: credentialVersion, SecurityUpdatedAt: securityUpdatedAt,
	}
}
