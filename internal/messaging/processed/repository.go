package processed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) IsProcessed(ctx context.Context, consumerName string, eventID uuid.UUID) (bool, error) {
	if r == nil || r.pool == nil {
		return false, errors.New("processed event repository is not configured")
	}
	consumerName = strings.TrimSpace(consumerName)
	if consumerName == "" {
		return false, errors.New("consumer name is required")
	}
	if eventID == uuid.Nil {
		return false, errors.New("event ID is required")
	}

	var processed bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM processed_events
			WHERE consumer_name = $1
			  AND event_id = $2
		)
	`, consumerName, eventID).Scan(&processed); err != nil {
		return false, fmt.Errorf("check processed event %s: %w", eventID, err)
	}
	return processed, nil
}

func (r *Repository) MarkProcessed(ctx context.Context, consumerName string, eventID uuid.UUID, metadata map[string]any) error {
	if r == nil || r.pool == nil {
		return errors.New("processed event repository is not configured")
	}
	consumerName = strings.TrimSpace(consumerName)
	if consumerName == "" {
		return errors.New("consumer name is required")
	}
	if eventID == uuid.Nil {
		return errors.New("event ID is required")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode processed event metadata: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO processed_events (consumer_name, event_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (consumer_name, event_id) DO NOTHING
	`, consumerName, eventID, encoded); err != nil {
		return fmt.Errorf("mark event %s processed: %w", eventID, err)
	}
	return nil
}
