package auditevent

import (
	"testing"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/google/uuid"
)

func TestEventFromEntryOmitsEmptyActorFields(t *testing.T) {
	entry := audit.Entry{ID: uuid.New(), TeamID: uuid.New(), ActorType: "system", Action: "test", ResourceType: "job", ResourceID: "1", Outcome: audit.OutcomeSuccess, Metadata: map[string]any{}, CreatedAt: time.Now()}
	event := eventFromEntry(entry)
	if event.ActorUserID != nil || event.ActorSessionID != nil || event.RequestID != nil {
		t.Fatalf("unexpected optional fields: %+v", event)
	}
}
