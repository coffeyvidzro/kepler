package teamtoken

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

type Repository struct{ queries *dbsqlc.Queries }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{queries: dbsqlc.New(db)} }

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	name, tokenHash, tokenPrefix string,
	permissions []string,
	createdBy uuid.UUID,
	expiresAt *time.Time,
) (Token, error) {
	row, err := r.queries.CreateTeamToken(
		ctx,
		dbsqlc.CreateTeamTokenParams{
			TeamID:      teamID,
			Name:        name,
			TokenHash:   tokenHash,
			TokenPrefix: tokenPrefix,
			Permissions: permissions,
			CreatedBy:   &createdBy,
			ExpiresAt:   pgconv.NullableTimestamptz(expiresAt),
		},
	)
	if err != nil {
		return Token{}, fmt.Errorf("create team token: %w", err)
	}
	return tokenFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]Token, error) {
	rows, err := r.queries.ListTeamTokens(ctx, dbsqlc.ListTeamTokensParams{TeamID: teamID})
	if err != nil {
		return nil, fmt.Errorf("list team tokens: %w", err)
	}
	tokens := make([]Token, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, tokenFromSQLC(row))
	}
	return tokens, nil
}

func (r *Repository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (Token, error) {
	row, err := r.queries.GetActiveTeamTokenByHash(
		ctx,
		dbsqlc.GetActiveTeamTokenByHashParams{TokenHash: tokenHash},
	)
	if err != nil {
		return Token{}, fmt.Errorf("get active team token by hash: %w", err)
	}
	return tokenFromSQLC(row), nil
}

func (r *Repository) Update(
	ctx context.Context,
	id, teamID uuid.UUID,
	name string,
	permissions []string,
	expiresAt *time.Time,
) (Token, error) {
	row, err := r.queries.UpdateTeamToken(
		ctx,
		dbsqlc.UpdateTeamTokenParams{
			ID:          id,
			TeamID:      teamID,
			Name:        name,
			Permissions: permissions,
			ExpiresAt:   pgconv.NullableTimestamptz(expiresAt),
		},
	)
	if err != nil {
		return Token{}, fmt.Errorf("update team token: %w", err)
	}
	return tokenFromSQLC(row), nil
}

func (r *Repository) Revoke(ctx context.Context, id, teamID uuid.UUID) (Token, error) {
	row, err := r.queries.RevokeTeamToken(ctx, dbsqlc.RevokeTeamTokenParams{ID: id, TeamID: teamID})
	if err != nil {
		return Token{}, fmt.Errorf("revoke team token: %w", err)
	}
	return tokenFromSQLC(row), nil
}

func (r *Repository) Touch(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.TouchTeamToken(ctx, dbsqlc.TouchTeamTokenParams{ID: id}); err != nil {
		return fmt.Errorf("touch team token: %w", err)
	}
	return nil
}

func tokenFromSQLC(row dbsqlc.TeamToken) Token {
	var createdBy *string
	if row.CreatedBy != nil {
		value := row.CreatedBy.String()
		createdBy = &value
	}
	return Token{
		ID:          row.ID.String(),
		TeamID:      row.TeamID.String(),
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
		Permissions: row.Permissions,
		CreatedBy:   createdBy,
		ExpiresAt:   pgconv.TimestamptzToTimePtr(row.ExpiresAt),
		RevokedAt:   pgconv.TimestamptzToTimePtr(row.RevokedAt),
		LastUsedAt:  pgconv.TimestamptzToTimePtr(row.LastUsedAt),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}
