package audit

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

type Event struct {
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]any
	Outcome      string
}

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

type RequestMetadata struct {
	RequestID string
	IPAddress string
	UserAgent string
}

type Entry struct {
	ID             uuid.UUID
	TeamID         uuid.UUID
	ActorType      string
	ActorUserID    uuid.UUID
	ActorSessionID string
	ActorTokenID   uuid.UUID
	Action         string
	ResourceType   string
	ResourceID     string
	Outcome        string
	Metadata       map[string]any
	Request        RequestMetadata
	CreatedAt      time.Time
}

type Sink interface {
	Record(context.Context, Entry) error
}

type sinkHolder struct{ sink Sink }

var configuredSink atomic.Pointer[sinkHolder]

func SetSink(sink Sink) {
	if sink == nil {
		configuredSink.Store(nil)
		return
	}
	configuredSink.Store(&sinkHolder{sink: sink})
}

type requestMetadataKey struct{}

func ContextWithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func Record(ctx context.Context, access tenant.AccessContext, event Event) {
	entry := Entry{TeamID: access.Scope.TeamID, ActorType: string(access.Actor.Type), ActorUserID: access.Actor.UserID, ActorSessionID: access.Actor.SessionID, ActorTokenID: access.Actor.TokenID, Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID, Outcome: event.Outcome, Metadata: event.Metadata}
	persist(ctx, entry)
	attributes := []slog.Attr{
		slog.String("actor_type", string(access.Actor.Type)),
		slog.String("team_id", access.Scope.TeamID.String()),
	}
	if access.Actor.UserID != uuid.Nil {
		attributes = append(attributes, slog.String("actor_user_id", access.Actor.UserID.String()))
	}
	if access.Actor.SessionID != "" {
		attributes = append(attributes, slog.String("actor_session_id", access.Actor.SessionID))
	}
	if access.Actor.TokenID != uuid.Nil {
		attributes = append(attributes, slog.String("actor_token_id", access.Actor.TokenID.String()))
	}
	record(ctx, event, attributes)
}

func RecordIdentity(ctx context.Context, userID uuid.UUID, event Event) {
	persist(ctx, Entry{ActorType: "user", ActorUserID: userID, Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID, Outcome: event.Outcome, Metadata: event.Metadata})
	record(ctx, event, []slog.Attr{
		slog.String("actor_type", "user"),
		slog.String("actor_user_id", userID.String()),
	})
}

func record(ctx context.Context, event Event, actor []slog.Attr) {
	outcome := event.Outcome
	if outcome == "" {
		outcome = OutcomeSuccess
	}
	attributes := []slog.Attr{
		slog.String("audit_action", event.Action),
		slog.String("resource_type", event.ResourceType),
		slog.String("resource_id", event.ResourceID),
		slog.String("outcome", outcome),
	}
	attributes = append(attributes, actor...)
	for key, value := range event.Metadata {
		attributes = append(attributes, slog.Any(key, value))
	}
	slog.LogAttrs(ctx, slog.LevelInfo, "security audit event", attributes...)
}

func persist(ctx context.Context, entry Entry) {
	if entry.Outcome == "" {
		entry.Outcome = OutcomeSuccess
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]any{}
	}
	if metadata, ok := ctx.Value(requestMetadataKey{}).(RequestMetadata); ok {
		entry.Request = metadata
	}
	if holder := configuredSink.Load(); holder != nil {
		if err := holder.sink.Record(ctx, entry); err != nil {
			slog.ErrorContext(ctx, "failed to persist security audit event", "audit_action", entry.Action, "error", err)
		}
	}
}
