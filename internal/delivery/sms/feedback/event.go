package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	attempt "github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
	feedback "github.com/coffeyvidzro/dugble/server/internal/delivery/feedback"
	"strings"
	"time"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const providerStatusEventType = "delivery_status"

func statusEvent(
	pending PendingMessage,
	response *smsapi.StatusResponse,
	observedAt time.Time,
) (feedback.Event, error) {
	if response == nil {
		return feedback.Event{}, errors.New("SMS provider returned an empty status response")
	}
	providerID := strings.ToLower(strings.TrimSpace(response.ProviderID))
	providerMessageID := strings.TrimSpace(response.ProviderMsgID)
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if providerID == "" {
		return feedback.Event{}, ErrProviderRequired
	}
	if providerMessageID == "" {
		return feedback.Event{}, ErrProviderMessageRequired
	}
	if providerID != strings.ToLower(strings.TrimSpace(pending.ProviderID)) ||
		providerMessageID != strings.TrimSpace(pending.ProviderMessageID) {
		return feedback.Event{}, errors.New("SMS provider status response does not match the pending attempt")
	}
	attemptStatus, err := deliveryAttemptStatus(status)
	if err != nil {
		return feedback.Event{}, err
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
		return feedback.Event{}, fmt.Errorf("encode SMS feedback metadata: %w", err)
	}
	errorCode, errorMessage := smsFeedbackError(status, providerStatus)
	event := feedback.Event{
		AttemptID:         pending.AttemptID,
		Provider:          providerID,
		ProviderEventID:   fmt.Sprintf("poll:%s:%d:%s", pending.AttemptID, pending.ReconcileAttempts+1, status),
		ProviderMessageID: providerMessageID,
		EventType:         providerStatusEventType + "." + status,
		Channel:           attempt.ChannelSMS,
		Status:            attemptStatus,
		ProviderStatus:    providerStatus,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		OccurredAt:        observedAt,
		ReceivedAt:        observedAt,
		Metadata:          metadata,
	}
	if err := event.Validate(); err != nil {
		return feedback.Event{}, err
	}
	return event, nil
}

func deliveryAttemptStatus(status string) (attempt.AttemptStatus, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case smsapi.StatusQueued:
		return attempt.StatusAccepted, nil
	case smsapi.StatusSubmitted:
		return attempt.StatusSubmitted, nil
	case smsapi.StatusSent:
		return attempt.StatusSent, nil
	case smsapi.StatusDelivered:
		return attempt.StatusDelivered, nil
	case smsapi.StatusUndelivered, smsapi.StatusFailed:
		return attempt.StatusPermanentFailure, nil
	case smsapi.StatusRejected:
		return attempt.StatusRejected, nil
	case smsapi.StatusExpired:
		return attempt.StatusExpired, nil
	case smsapi.StatusUnknown:
		return attempt.StatusUnknown, nil
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
