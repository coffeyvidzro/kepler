package user

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
)

type Repository struct {
	queries *dbsqlc.Queries
}

func (r *Repository) GetNotificationRecipient(ctx context.Context, userID uuid.UUID) (notifications.Recipient, error) {
	user, err := r.GetByID(ctx, userID.String())
	if err != nil {
		return notifications.Recipient{}, err
	}
	return notifications.Recipient{Name: user.Name, Email: user.Email}, nil
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
}

func (r *Repository) GetByID(ctx context.Context, id string) (User, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return User{}, fmt.Errorf("parse user id: %w", err)
	}

	row, err := r.queries.GetUserByID(ctx, dbsqlc.GetUserByIDParams{ID: parsedID})
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}

	return userFromSQLC(row), nil
}

func (r *Repository) UpdateProfile(ctx context.Context, id string, name string) (User, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return User{}, fmt.Errorf("parse user id: %w", err)
	}

	row, err := r.queries.UpdateUserProfile(
		ctx,
		dbsqlc.UpdateUserProfileParams{ID: parsedID, Name: name},
	)
	if err != nil {
		return User{}, fmt.Errorf("update user profile: %w", err)
	}

	return userFromSQLC(row), nil
}

func (r *Repository) UpdateEmail(ctx context.Context, id string, email string) (User, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return User{}, fmt.Errorf("parse user id: %w", err)
	}

	row, err := r.queries.UpdateUserEmail(
		ctx,
		dbsqlc.UpdateUserEmailParams{ID: parsedID, Email: email},
	)
	if err != nil {
		return User{}, fmt.Errorf("update user email: %w", err)
	}

	return userFromSQLC(row), nil
}

func (r *Repository) UpdatePassword(
	ctx context.Context,
	id string,
	passwordHash string,
) (User, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return User{}, fmt.Errorf("parse user id: %w", err)
	}

	row, err := r.queries.UpdateUserPassword(
		ctx,
		dbsqlc.UpdateUserPasswordParams{ID: parsedID, PasswordHash: &passwordHash},
	)
	if err != nil {
		return User{}, fmt.Errorf("update user password: %w", err)
	}

	return userFromSQLC(row), nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	return r.queries.DeleteUser(ctx, dbsqlc.DeleteUserParams{ID: parsedID})
}

func userFromSQLC(row dbsqlc.User) User {
	return User{
		ID:            row.ID.String(),
		Email:         row.Email,
		EmailVerified: row.EmailVerified,
		Name:          row.Name,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}
