package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

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
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
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

	_, err := r.queries.CreateVerificationToken(ctx, dbsqlc.CreateVerificationTokenParams{
		Identifier: identifier,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
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
	row, err := r.queries.ResetPasswordWithToken(ctx, dbsqlc.ResetPasswordWithTokenParams{
		Email: email, Identifier: identifier, TokenHash: tokenHash, PasswordHash: &passwordHash,
	})
	if err != nil {
		return UserRecord{}, fmt.Errorf("reset password with token: %w", err)
	}
	return userRecordFromValues(row.ID, row.Email, row.EmailVerified, row.Name, row.PasswordHash, row.CreatedAt.Time, row.UpdatedAt.Time, row.CredentialVersion, row.SecurityUpdatedAt.Time), nil
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
