package audit

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestRecordIncludesActorTenantAndResource(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	teamID, userID, tokenID := uuid.New(), uuid.New(), uuid.New()
	Record(context.Background(), tenant.AccessContext{
		Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: userID, SessionID: "session-1", TokenID: tokenID},
		Scope: tenant.Scope{TeamID: teamID},
	}, Event{Action: "team.updated", ResourceType: "team", ResourceID: teamID.String()})

	for _, value := range []string{"security audit event", "team.updated", teamID.String(), userID.String(), tokenID.String(), "session-1"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("audit output does not contain %q: %s", value, output.String())
		}
	}
}

func TestRecordIdentityIncludesUser(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	userID := uuid.New()
	RecordIdentity(context.Background(), userID, Event{Action: "identity.password_reset", ResourceType: "user", ResourceID: userID.String()})
	for _, value := range []string{"identity.password_reset", userID.String(), "success"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("identity audit output does not contain %q: %s", value, output.String())
		}
	}
}

type captureSink struct{ entries []Entry }

func (s *captureSink) Record(_ context.Context, entry Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func TestRecordPersistsStructuredEntry(t *testing.T) {
	sink := &captureSink{}
	SetSink(sink)
	t.Cleanup(func() { SetSink(nil) })
	teamID, userID := uuid.New(), uuid.New()
	ctx := ContextWithRequestMetadata(context.Background(), RequestMetadata{RequestID: "request-1", IPAddress: "192.0.2.1", UserAgent: "test"})
	Record(ctx, tenant.AccessContext{Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: userID}, Scope: tenant.Scope{TeamID: teamID}}, Event{Action: "team.updated", ResourceType: "team", ResourceID: teamID.String(), Metadata: map[string]any{"field": "name"}})
	if len(sink.entries) != 1 {
		t.Fatalf("persisted %d entries", len(sink.entries))
	}
	entry := sink.entries[0]
	if entry.TeamID != teamID || entry.ActorUserID != userID || entry.Request.RequestID != "request-1" || entry.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}
