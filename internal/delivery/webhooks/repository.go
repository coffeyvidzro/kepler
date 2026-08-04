package webhookdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrClaimLost = errors.New("webhook delivery claim was lost")

type deliveryQueries interface {
	ClaimWebhookDeliveries(context.Context, dbsqlc.ClaimWebhookDeliveriesParams) ([]dbsqlc.ClaimWebhookDeliveriesRow, error)
	MarkWebhookDeliverySucceeded(context.Context, dbsqlc.MarkWebhookDeliverySucceededParams) (dbsqlc.MarkWebhookDeliverySucceededRow, error)
	ScheduleWebhookDeliveryRetry(context.Context, dbsqlc.ScheduleWebhookDeliveryRetryParams) (dbsqlc.WebhookDelivery, error)
	MarkWebhookDeliveryFailed(context.Context, dbsqlc.MarkWebhookDeliveryFailedParams) (dbsqlc.MarkWebhookDeliveryFailedRow, error)
	ReleaseWebhookDeliveryClaim(context.Context, dbsqlc.ReleaseWebhookDeliveryClaimParams) (int64, error)
}

type Repository struct {
	queries          deliveryQueries
	autoDisableAfter int32
}

type RepositoryConfig struct {
	AutoDisableAfter int32
}

func NewRepository(db *pgxpool.Pool, configs ...RepositoryConfig) *Repository {
	config := RepositoryConfig{AutoDisableAfter: 20}
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.AutoDisableAfter <= 0 {
		config.AutoDisableAfter = 20
	}
	if db == nil {
		return &Repository{autoDisableAfter: config.AutoDisableAfter}
	}
	return &Repository{queries: dbsqlc.New(db), autoDisableAfter: config.AutoDisableAfter}
}

func (r *Repository) Claim(ctx context.Context, workerID string, limit int32, staleBefore time.Time) ([]ClaimedDelivery, error) {
	workerID, err := r.validate(workerID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New("webhook delivery claim limit must be positive")
	}

	rows, err := r.queries.ClaimWebhookDeliveries(ctx, dbsqlc.ClaimWebhookDeliveriesParams{
		WorkerID: &workerID, StaleBefore: pgconv.NullableTimestamptz(&staleBefore), LimitCount: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", err)
	}
	deliveries := make([]ClaimedDelivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, ClaimedDelivery{
			ID: row.ID, EventID: row.EventID, EndpointID: row.EndpointID, AttemptCount: row.AttemptCount,
			TeamID: row.TeamID, EventType: row.EventType, Payload: json.RawMessage(row.Payload),
			OccurredAt: pgconv.TimestamptzToTime(row.OccurredAt), URL: row.Url, SigningSecret: row.SigningSecret,
		})
	}
	return deliveries, nil
}

func (r *Repository) MarkSucceeded(ctx context.Context, id uuid.UUID, workerID string, status int32, body *string) error {
	workerID, err := r.validateResult(id, workerID)
	if err != nil {
		return err
	}
	_, err = r.queries.MarkWebhookDeliverySucceeded(ctx, dbsqlc.MarkWebhookDeliverySucceededParams{
		ID: id, WorkerID: &workerID, ResponseStatus: &status, ResponseBody: body,
	})
	return resultError("mark webhook delivery succeeded", err)
}

func (r *Repository) ScheduleRetry(ctx context.Context, id uuid.UUID, workerID string, nextAttempt time.Time, status *int32, body *string, lastError string) error {
	workerID, err := r.validateResult(id, workerID)
	if err != nil {
		return err
	}
	if nextAttempt.IsZero() {
		return errors.New("webhook delivery next attempt is required")
	}
	_, err = r.queries.ScheduleWebhookDeliveryRetry(ctx, dbsqlc.ScheduleWebhookDeliveryRetryParams{
		ID: id, WorkerID: &workerID, NextAttemptAt: pgconv.NullableTimestamptz(&nextAttempt),
		ResponseStatus: status, ResponseBody: body, LastError: &lastError,
	})
	return resultError("schedule webhook delivery retry", err)
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, workerID string, status *int32, body *string, lastError string) error {
	workerID, err := r.validateResult(id, workerID)
	if err != nil {
		return err
	}
	_, err = r.queries.MarkWebhookDeliveryFailed(ctx, dbsqlc.MarkWebhookDeliveryFailedParams{
		ID: id, WorkerID: &workerID, ResponseStatus: status, ResponseBody: body, LastError: &lastError,
		AutoDisableAfter: r.autoDisableAfter,
	})
	return resultError("mark webhook delivery failed", err)
}

func (r *Repository) ReleaseClaim(ctx context.Context, id uuid.UUID, workerID string) error {
	workerID, err := r.validateResult(id, workerID)
	if err != nil {
		return err
	}
	count, err := r.queries.ReleaseWebhookDeliveryClaim(ctx, dbsqlc.ReleaseWebhookDeliveryClaimParams{ID: id, WorkerID: &workerID})
	if err != nil {
		return fmt.Errorf("release webhook delivery claim: %w", err)
	}
	if count != 1 {
		return ErrClaimLost
	}
	return nil
}

func (r *Repository) validate(workerID string) (string, error) {
	if r == nil || r.queries == nil {
		return "", errors.New("webhook delivery repository is not configured")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return "", errors.New("webhook delivery worker id is required")
	}
	return workerID, nil
}

func (r *Repository) validateResult(id uuid.UUID, workerID string) (string, error) {
	workerID, err := r.validate(workerID)
	if err != nil {
		return "", err
	}
	if id == uuid.Nil {
		return "", errors.New("webhook delivery id is required")
	}
	return workerID, nil
}

func resultError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClaimLost
	}
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
