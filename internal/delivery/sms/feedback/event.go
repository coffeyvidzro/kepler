package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformdelivery "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
	platformfeedback "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/feedback"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const providerStatusEventType = "delivery_status"

func statusEvent(
	pending PendingMessage,
	response *smsapi.StatusResponse,
	observedAt time.Time,
) (platformfeedback.Event, error) {
	if response == nil {
		return platformfeedback.Event{}, errors.New("SMS provider returned an empty status response")
	}
	providerID := strings.ToLower(strings.TrimSpace(response.ProviderID))
	providerMessageID := strings.TrimSpace(response.ProviderMsgID)
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if providerID == "" {
		return platformfeedback.Event{}, ErrProviderRequired
	}
	if providerMessageID == "" {
		return platformfeedback.Event{}, ErrProviderMessageRequired
	}
	if providerID != strings.ToLower(strings.TrimSpace(pending.ProviderID)) ||
		providerMessageID != strings.TrimSpace(pending.ProviderMessageID) {
		return platformfeedback.Event{}, errors.New("SMS provider status response does not match the pending attempt")
	}
	attemptStatus, err := deliveryAttemptStatus(status)
	if err != nil {
		return platformfeedback.Event{}, err
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	providerStatus := strings.TrimSpace(response.ProviderStatus)
	if providerStatus == "" {
		providerStatus = status
	}
	metadata, err := json.Marshal(map[string]any{
		"source":            "poll",
		"normalized_status": status,
		"provider_status":   providerStatus,
	})
	if err != nil {
		return platformfeedback.Event{}, fmt.Errorf("encode SMS feedback metadata: %w", err)
	}
	errorCode, errorMessage := smsFeedbackError(status, providerStatus)
	event := platformfeedback.Event{
		AttemptID:         pending.AttemptID,
		Provider:          providerID,
		ProviderEventID:   fmt.Sprintf("poll:%s:%d:%s", pending.AttemptID, pending.ReconcileAttempts+1, status),
		ProviderMessageID: providerMessageID,
		EventType:         providerStatusEventType + "." + status,
		Channel:           messaging.ChannelSMS,
		Status:            attemptStatus,
		ProviderStatus:    providerStatus,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		OccurredAt:        observedAt,
		ReceivedAt:        observedAt,
		Metadata:          metadata,
	}
	if err := event.Validate(); err != nil {
		return platformfeedback.Event{}, err
	}
	return event, nil
}

func deliveryAttemptStatus(status string) (platformdelivery.AttemptStatus, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case smsapi.StatusQueued:
		return platformdelivery.StatusAccepted, nil
	case smsapi.StatusSubmitted:
		return platformdelivery.StatusSubmitted, nil
	case smsapi.StatusSent:
		return platformdelivery.StatusSent, nil
	case smsapi.StatusDelivered:
		return platformdelivery.StatusDelivered, nil
	case smsapi.StatusUndelivered, smsapi.StatusFailed:
		return platformdelivery.StatusPermanentFailure, nil
	case smsapi.StatusRejected:
		return platformdelivery.StatusRejected, nil
	case smsapi.StatusExpired:
		return platformdelivery.StatusExpired, nil
	case smsapi.StatusUnknown:
		return platformdelivery.StatusUnknown, nil
	default:
		return "", ErrUnsupportedStatus
	}
}

func smsFeedbackError(status, providerStatus string) (string, string) {
	switch status {
	case smsapi.StatusUndelivered:
		return "sms_undelivered", providerStatus
	case smsapi.StatusRejected:
		return "sms_rejected", providerStatus
	case smsapi.StatusFailed:
		return "sms_failed", providerStatus
	case smsapi.StatusExpired:
		return "sms_expired", providerStatus
	case smsapi.StatusUnknown:
		return "sms_unknown", providerStatus
	default:
		return "", ""
	}
}
