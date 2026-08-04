package audit

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func Record(ctx context.Context, access tenant.AccessContext, event Event) {
	actor := actorFromAccess(access)
	entry := newEntry(event)
	entry.TeamID = access.Scope.TeamID
	actor.apply(&entry)
	persist(ctx, entry)

	attributes := actor.attributes()
	attributes = append(attributes, slog.String("team_id", access.Scope.TeamID.String()))
	logEvent(ctx, event, attributes)
}

func RecordIdentity(ctx context.Context, userID uuid.UUID, event Event) {
	actor := identityActor(userID)
	entry := newEntry(event)
	actor.apply(&entry)
	persist(ctx, entry)
	logEvent(ctx, event, actor.attributes())
}

func logEvent(ctx context.Context, event Event, actorAttributes []slog.Attr) {
	if ctx == nil {
		ctx = context.Background()
	}
	attributes := []slog.Attr{
		slog.String("audit_action", event.Action),
		slog.String("resource_type", event.ResourceType),
		slog.String("resource_id", event.ResourceID),
		slog.String("outcome", normalizedOutcome(event.Outcome)),
	}
	attributes = append(attributes, actorAttributes...)
	for key, value := range event.Metadata {
		attributes = append(attributes, slog.Any(key, value))
	}
	slog.LogAttrs(ctx, slog.LevelInfo, "security audit event", attributes...)
}
