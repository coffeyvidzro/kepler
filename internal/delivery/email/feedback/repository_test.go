package feedback

import (
	"testing"
	"time"
)

func TestRecipientStatusTransition(t *testing.T) {
	occurredAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		current    string
		eventType  string
		wantStatus string
		wantApply  bool
		wantError  bool
	}{
		{name: "send marks pending submitted", current: "pending", eventType: "send", wantStatus: "submitted", wantApply: true},
		{name: "delivery after submitted", current: "submitted", eventType: "delivery", wantStatus: "delivered", wantApply: true},
		{name: "delivery cannot revive bounce", current: "bounced", eventType: "delivery", wantApply: false},
		{name: "bounce overrides delivery", current: "delivered", eventType: "bounce", wantStatus: "bounced", wantApply: true},
		{name: "complaint overrides bounce", current: "bounced", eventType: "complaint", wantStatus: "complained", wantApply: true},
		{name: "delay cannot downgrade delivery", current: "delivered", eventType: "delivery_delay", wantApply: false},
		{name: "reject cannot override delivery", current: "delivered", eventType: "reject", wantApply: false},
		{name: "reject becomes rejected", current: "submitted", eventType: "reject", wantStatus: "rejected", wantApply: true},
		{name: "rendering failure becomes failed", current: "submitted", eventType: "rendering_failure", wantStatus: "failed", wantApply: true},
		{name: "unknown event", current: "submitted", eventType: "unknown", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transition, apply, err := recipientStatusTransition(tt.current, tt.eventType, occurredAt)
			if (err != nil) != tt.wantError {
				t.Fatalf("recipientStatusTransition() error = %v, wantError %v", err, tt.wantError)
			}
			if apply != tt.wantApply {
				t.Fatalf("recipientStatusTransition() apply = %v, want %v", apply, tt.wantApply)
			}
			if transition.status != tt.wantStatus {
				t.Fatalf("recipientStatusTransition() status = %q, want %q", transition.status, tt.wantStatus)
			}
		})
	}
}

func TestRecipientAggregateRules(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []string
		wantStatus string
	}{
		{name: "all delivered", statuses: []string{"delivered", "delivered"}, wantStatus: "delivered"},
		{name: "some delivered", statuses: []string{"delivered", "submitted"}, wantStatus: "partially_delivered"},
		{name: "delivery mixed with bounce", statuses: []string{"delivered", "bounced"}, wantStatus: "partially_delivered"},
		{name: "complaint wins", statuses: []string{"delivered", "complained"}, wantStatus: "complained"},
		{name: "all bounced", statuses: []string{"bounced", "bounced"}, wantStatus: "bounced"},
		{name: "mixed terminal failures", statuses: []string{"bounced", "rejected"}, wantStatus: "partially_failed"},
		{name: "failure with unresolved recipient", statuses: []string{"bounced", "submitted"}, wantStatus: "partially_failed"},
		{name: "delay while unresolved", statuses: []string{"delayed", "submitted"}, wantStatus: "delayed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counts := make(map[string]int, len(tt.statuses))
			for _, status := range tt.statuses {
				counts[status]++
			}
			transition := aggregateTransitionFromCounts(counts, len(tt.statuses), "submitted", nil, nil)
			if transition.status != tt.wantStatus {
				t.Fatalf("aggregate status = %q, want %q", transition.status, tt.wantStatus)
			}
		})
	}
}
