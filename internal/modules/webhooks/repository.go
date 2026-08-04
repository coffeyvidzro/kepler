package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) InTransaction(ctx context.Context, operation func(pgx.Tx) error) error {
	return database.InTransaction(ctx, r.db, operation)
}

func (r *Repository) CreateEndpoint(ctx context.Context, teamID uuid.UUID, endpoint validatedEndpoint, secret []byte) (Endpoint, error) {
	row, err := r.queries.CreateWebhookEndpoint(ctx, dbsqlc.CreateWebhookEndpointParams{
		TeamID: teamID, Url: endpoint.URL, SigningSecret: secret, SubscribedEvents: endpoint.SubscribedEvents,
	})
	if err != nil {
		return Endpoint{}, fmt.Errorf("create webhook endpoint: %w", err)
	}
	return endpointFromSQLC(row), nil
}

func (r *Repository) ListEndpoints(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Endpoint, error) {
	rows, err := r.queries.ListWebhookEndpoints(ctx, dbsqlc.ListWebhookEndpointsParams{
		TeamID: teamID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	endpoints := make([]Endpoint, 0, len(rows))
	for _, row := range rows {
		endpoints = append(endpoints, endpointFromSQLC(row))
	}
	return endpoints, nil
}

func (r *Repository) GetEndpoint(ctx context.Context, id, teamID uuid.UUID) (Endpoint, error) {
	row, err := r.queries.GetWebhookEndpoint(ctx, dbsqlc.GetWebhookEndpointParams{ID: id, TeamID: teamID})
	if err != nil {
		return Endpoint{}, fmt.Errorf("get webhook endpoint: %w", err)
	}
	return endpointFromSQLC(row), nil
}

func (r *Repository) UpdateEndpoint(ctx context.Context, id, teamID uuid.UUID, endpoint validatedEndpoint) (Endpoint, error) {
	var row dbsqlc.WebhookEndpoint
	err := r.InTransaction(ctx, func(tx pgx.Tx) error {
		queries := r.queries.WithTx(tx)
		updated, err := queries.UpdateWebhookEndpoint(ctx, dbsqlc.UpdateWebhookEndpointParams{
			ID: id, TeamID: teamID, Url: endpoint.URL, Enabled: endpoint.Enabled, SubscribedEvents: endpoint.SubscribedEvents,
		})
		if err != nil {
			return fmt.Errorf("update webhook endpoint: %w", err)
		}
		row = updated
		if endpoint.Enabled {
			return nil
		}
		if _, err := queries.CancelWebhookDeliveriesForEndpoint(ctx, dbsqlc.CancelWebhookDeliveriesForEndpointParams{EndpointID: id}); err != nil {
			return fmt.Errorf("cancel webhook endpoint deliveries: %w", err)
		}
		return nil
	})
	if err != nil {
		return Endpoint{}, err
	}
	return endpointFromSQLC(row), nil
}

func (r *Repository) DisableEndpoint(ctx context.Context, id, teamID uuid.UUID) (Endpoint, error) {
	var row dbsqlc.WebhookEndpoint
	err := r.InTransaction(ctx, func(tx pgx.Tx) error {
		queries := r.queries.WithTx(tx)
		disabled, err := queries.DisableWebhookEndpoint(ctx, dbsqlc.DisableWebhookEndpointParams{ID: id, TeamID: teamID})
		if err != nil {
			return fmt.Errorf("disable webhook endpoint: %w", err)
		}
		row = disabled
		if _, err := queries.CancelWebhookDeliveriesForEndpoint(ctx, dbsqlc.CancelWebhookDeliveriesForEndpointParams{EndpointID: id}); err != nil {
			return fmt.Errorf("cancel webhook endpoint deliveries: %w", err)
		}
		return nil
	})
	if err != nil {
		return Endpoint{}, err
	}
	return endpointFromSQLC(row), nil
}

func (r *Repository) RotateSecret(ctx context.Context, id, teamID uuid.UUID, secret []byte) (Endpoint, error) {
	row, err := r.queries.RotateWebhookEndpointSecret(ctx, dbsqlc.RotateWebhookEndpointSecretParams{
		ID: id, TeamID: teamID, SigningSecret: secret,
	})
	if err != nil {
		return Endpoint{}, fmt.Errorf("rotate webhook endpoint secret: %w", err)
	}
	return endpointFromSQLC(row), nil
}

func (r *Repository) ListEvents(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Event, error) {
	rows, err := r.queries.ListWebhookEvents(ctx, dbsqlc.ListWebhookEventsParams{
		TeamID: teamID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list webhook events: %w", err)
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, eventFromSQLC(row))
	}
	return events, nil
}

func (r *Repository) GetEvent(ctx context.Context, id, teamID uuid.UUID) (Event, error) {
	row, err := r.queries.GetWebhookEvent(ctx, dbsqlc.GetWebhookEventParams{ID: id, TeamID: teamID})
	if err != nil {
		return Event{}, fmt.Errorf("get webhook event: %w", err)
	}
	return eventFromSQLC(row), nil
}

func (r *Repository) GetDelivery(ctx context.Context, id, teamID uuid.UUID) (Delivery, error) {
	row, err := r.queries.GetWebhookDelivery(ctx, dbsqlc.GetWebhookDeliveryParams{ID: id, TeamID: teamID})
	if err != nil {
		return Delivery{}, fmt.Errorf("get webhook delivery: %w", err)
	}
	return deliveryFromSQLC(row), nil
}

func (r *Repository) RetryDelivery(ctx context.Context, id, teamID uuid.UUID) (Delivery, error) {
	row, err := r.queries.RetryWebhookDelivery(ctx, dbsqlc.RetryWebhookDeliveryParams{ID: id, TeamID: teamID})
	if err != nil {
		return Delivery{}, fmt.Errorf("retry webhook delivery: %w", err)
	}
	return deliveryFromSQLC(row), nil
}

func (r *Repository) CreateEventTx(ctx context.Context, tx pgx.Tx, event platformwebhook.Event) (uuid.UUID, error) {
	row, err := r.queries.WithTx(tx).CreateWebhookEvent(ctx, dbsqlc.CreateWebhookEventParams{
		ID: event.ID, TeamID: event.TeamID, EventType: event.Type, ObjectType: event.ObjectType,
		ObjectID: event.ObjectID, Payload: event.Payload, OccurredAt: pgconv.NullableTimestamptz(&event.OccurredAt),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create webhook event: %w", err)
	}
	return row.ID, nil
}

func (r *Repository) CreateDeliveriesForEventTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, nextAttemptAt time.Time) (int64, error) {
	count, err := r.queries.WithTx(tx).CreateWebhookDeliveriesForEvent(ctx, dbsqlc.CreateWebhookDeliveriesForEventParams{
		EventID: eventID, NextAttemptAt: pgconv.NullableTimestamptz(&nextAttemptAt),
	})
	if err != nil {
		return 0, fmt.Errorf("create webhook deliveries: %w", err)
	}
	return count, nil
}

func (r *Repository) CreateDeliveryTx(ctx context.Context, tx pgx.Tx, eventID, endpointID uuid.UUID, nextAttemptAt time.Time) (uuid.UUID, error) {
	row, err := r.queries.WithTx(tx).CreateWebhookDelivery(ctx, dbsqlc.CreateWebhookDeliveryParams{
		EventID: eventID, EndpointID: endpointID, NextAttemptAt: pgconv.NullableTimestamptz(&nextAttemptAt),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create webhook delivery: %w", err)
	}
	return row.ID, nil
}

func endpointFromSQLC(row dbsqlc.WebhookEndpoint) Endpoint {
	return Endpoint{
		ID: row.ID.String(), TeamID: row.TeamID.String(), URL: row.Url, Enabled: row.Enabled,
		SubscribedEvents:    row.SubscribedEvents,
		CreatedAt:           pgconv.TimestamptzToTime(row.CreatedAt),
		UpdatedAt:           pgconv.TimestamptzToTime(row.UpdatedAt),
		DisabledAt:          pgconv.TimestamptzToTimePtr(row.DisabledAt),
		ConsecutiveFailures: row.ConsecutiveFailures,
		LastFailureAt:       pgconv.TimestamptzToTimePtr(row.LastFailureAt),
		DisabledReason:      row.DisabledReason,
	}
}

func eventFromSQLC(row dbsqlc.WebhookEvent) Event {
	var objectID *string
	if row.ObjectID != nil {
		value := row.ObjectID.String()
		objectID = &value
	}
	return Event{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Type: row.EventType, ObjectType: row.ObjectType,
		ObjectID: objectID, Payload: json.RawMessage(row.Payload),
		OccurredAt: pgconv.TimestamptzToTime(row.OccurredAt), CreatedAt: pgconv.TimestamptzToTime(row.CreatedAt),
	}
}

func deliveryFromSQLC(row dbsqlc.WebhookDelivery) Delivery {
	return Delivery{
		ID: row.ID.String(), EventID: row.EventID.String(), EndpointID: row.EndpointID.String(), Status: row.Status,
		AttemptCount: row.AttemptCount, NextAttemptAt: pgconv.TimestamptzToTime(row.NextAttemptAt),
		LastAttemptAt:  pgconv.TimestamptzToTimePtr(row.LastAttemptAt),
		ResponseStatus: row.ResponseStatus, ResponseBody: row.ResponseBody, LastError: row.LastError,
		DeliveredAt: pgconv.TimestamptzToTimePtr(row.DeliveredAt),
		CreatedAt:   pgconv.TimestamptzToTime(row.CreatedAt), UpdatedAt: pgconv.TimestamptzToTime(row.UpdatedAt),
	}
}
