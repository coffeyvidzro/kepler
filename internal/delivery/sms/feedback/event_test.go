package feedback

import (
	"testing"
	"time"

	"github.com/google/uuid"

	platformdelivery "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestStatusEventMapsProviderStates(t *testing.T) {
	t.Parallel()

	attemptID := uuid.New()
	pending := PendingMessage{
		AttemptID:         attemptID,
		ProviderID:        "mnotify",
		ProviderMessageID: "provider-message",
		ReconcileAttempts: 2,
	}
	tests := []struct {
		status string
		want   platformdelivery.AttemptStatus
	}{
		{status: smsapi.StatusQueued, want: platformdelivery.StatusAccepted},
		{status: smsapi.StatusSubmitted, want: platformdelivery.StatusSubmitted},
		{status: smsapi.StatusSent, want: platformdelivery.StatusSent},
		{status: smsapi.StatusDelivered, want: platformdelivery.StatusDelivered},
		{status: smsapi.StatusUndelivered, want: platformdelivery.StatusPermanentFailure},
		{status: smsapi.StatusRejected, want: platformdelivery.StatusRejected},
		{status: smsapi.StatusExpired, want: platformdelivery.StatusExpired},
		{status: smsapi.StatusUnknown, want: platformdelivery.StatusUnknown},
	}
	for _, test := range tests {
		test := test
		t.Run(test.status, func(t *testing.T) {
			t.Parallel()
			event, err := statusEvent(pending, &smsapi.StatusResponse{
				ProviderID:    pending.ProviderID,
				ProviderMsgID: pending.ProviderMessageID,
				Status:        test.status,
			}, time.Now().UTC())
			if err != nil {
				t.Fatalf("statusEvent() error = %v", err)
			}
			if event.Status != test.want {
				t.Fatalf("statusEvent() status = %s, want %s", event.Status, test.want)
			}
			if event.AttemptID != attemptID || event.ProviderEventID != "poll:"+attemptID.String()+":3:"+test.status {
				t.Fatalf("statusEvent() identity = %+v", event)
			}
		})
	}
}
