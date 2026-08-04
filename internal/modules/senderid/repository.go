package senderid

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrSenderIDAlreadyExists = errors.New("sender id already exists")

type Repository struct{ queries *dbsqlc.Queries }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{queries: dbsqlc.New(db)} }

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	name string,
	countryCode string,
	purpose string,
	provider *string,
	createdBy uuid.UUID,
) (SenderID, error) {
	row, err := r.queries.CreateSenderID(ctx, dbsqlc.CreateSenderIDParams{
		TeamID:      teamID,
		Name:        name,
		CountryCode: countryCode,
		Purpose:     purpose,
		Provider:    provider,
		CreatedBy:   &createdBy,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return SenderID{}, ErrSenderIDAlreadyExists
		}
		return SenderID{}, fmt.Errorf("create sender id: %w", err)
	}
	return senderIDFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderID, error) {
	rows, err := r.queries.ListSenderIDs(ctx, dbsqlc.ListSenderIDsParams{TeamID: teamID})
	if err != nil {
		return nil, fmt.Errorf("list sender ids: %w", err)
	}
	senderIDs := make([]SenderID, 0, len(rows))
	for _, row := range rows {
		senderIDs = append(senderIDs, senderIDFromSQLC(row))
	}
	return senderIDs, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	row, err := r.queries.GetSenderID(ctx, dbsqlc.GetSenderIDParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id: %w", err)
	}
	return senderIDFromSQLC(row), nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	row, err := r.queries.DeleteSenderID(ctx, dbsqlc.DeleteSenderIDParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderID{}, fmt.Errorf("delete sender id: %w", err)
	}
	return senderIDFromSQLC(row), nil
}

func senderIDFromSQLC(row dbsqlc.SenderID) SenderID {
	var createdBy *string
	if row.CreatedBy != nil {
		value := row.CreatedBy.String()
		createdBy = &value
	}
	return SenderID{
		ID:              row.ID.String(),
		TeamID:          row.TeamID.String(),
		Name:            row.Name,
		CountryCode:     row.CountryCode,
		Purpose:         row.Purpose,
		Status:          row.Status,
		Provider:        row.Provider,
		RejectionReason: row.RejectionReason,
		ApprovedAt:      pgconv.TimestamptzToTimePtr(row.ApprovedAt),
		RejectedAt:      pgconv.TimestamptzToTimePtr(row.RejectedAt),
		SuspendedAt:     pgconv.TimestamptzToTimePtr(row.SuspendedAt),
		CreatedBy:       createdBy,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
