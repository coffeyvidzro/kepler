package topic

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

type Repository struct {
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
}

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Topic, error) {
	row, err := r.queries.CreateTopic(ctx, dbsqlc.CreateTopicParams{
		TeamID:              teamID,
		Name:                req.Name,
		Description:         req.Description,
		DefaultSubscription: req.DefaultSubscription,
		Visibility:          req.Visibility,
	})
	if err != nil {
		return Topic{}, fmt.Errorf("create topic: %w", err)
	}
	return topicFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Topic, error) {
	rows, err := r.queries.ListTopics(ctx, dbsqlc.ListTopicsParams{
		TeamID:     teamID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	return topicsFromSQLC(rows), nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Topic, error) {
	row, err := r.queries.GetTopic(ctx, dbsqlc.GetTopicParams{ID: id, TeamID: teamID})
	if err != nil {
		return Topic{}, err
	}
	return topicFromSQLC(row), nil
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, name string, description *string, visibility string) (Topic, error) {
	row, err := r.queries.UpdateTopic(ctx, dbsqlc.UpdateTopicParams{
		Name:        name,
		Description: description,
		Visibility:  visibility,
		ID:          id,
		TeamID:      teamID,
	})
	if err != nil {
		return Topic{}, err
	}
	return topicFromSQLC(row), nil
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Topic, error) {
	row, err := r.queries.DeleteTopic(ctx, dbsqlc.DeleteTopicParams{ID: id, TeamID: teamID})
	if err != nil {
		return Topic{}, err
	}
	return topicFromSQLC(row), nil
}

func (r *Repository) CursorExists(ctx context.Context, teamID, cursorID uuid.UUID) (bool, error) {
	return r.queries.TopicCursorExists(ctx, dbsqlc.TopicCursorExistsParams{
		CursorID: cursorID,
		TeamID:   teamID,
	})
}

func (r *Repository) ListPage(ctx context.Context, teamID uuid.UUID, limit int32, after, before *uuid.UUID) ([]Topic, error) {
	var (
		rows []dbsqlc.Topic
		err  error
	)

	switch {
	case after != nil:
		rows, err = r.queries.ListTopicsAfter(ctx, dbsqlc.ListTopicsAfterParams{
			ScopeTeamID: teamID,
			CursorID:    *after,
			PageLimit:   limit,
		})
	case before != nil:
		rows, err = r.queries.ListTopicsBefore(ctx, dbsqlc.ListTopicsBeforeParams{
			ScopeTeamID: teamID,
			CursorID:    *before,
			PageLimit:   limit,
		})
	default:
		rows, err = r.queries.ListTopics(ctx, dbsqlc.ListTopicsParams{
			TeamID:     teamID,
			PageOffset: 0,
			PageLimit:  limit,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list topic page: %w", err)
	}
	return topicsFromSQLC(rows), nil
}

func topicsFromSQLC(rows []dbsqlc.Topic) []Topic {
	values := make([]Topic, 0, len(rows))
	for _, row := range rows {
		values = append(values, topicFromSQLC(row))
	}
	return values
}

func topicFromSQLC(row dbsqlc.Topic) Topic {
	return Topic{
		ID:                  row.ID.String(),
		TeamID:              row.TeamID.String(),
		Name:                row.Name,
		Description:         row.Description,
		DefaultSubscription: row.DefaultSubscription,
		Visibility:          row.Visibility,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}
