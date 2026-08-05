package delivery

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
)

type AttemptStatus string

const (
	StatusClaimed          AttemptStatus = "claimed"
	StatusRequestStarted   AttemptStatus = "request_started"
	StatusSubmissionUnknown AttemptStatus = "submission_unknown"
	StatusSubmitted        AttemptStatus = "submitted"
	StatusAccepted         AttemptStatus = "accepted"
	StatusSent             AttemptStatus = "sent"
	StatusDelivered        AttemptStatus = "delivered"
	StatusRetryableFailure AttemptStatus = "retryable_failure"
	StatusPermanentFailure AttemptStatus = "permanent_failure"
	StatusRejected         AttemptStatus = "rejected"
	StatusExpired          AttemptStatus = "expired"
	StatusCanceled         AttemptStatus = "canceled"
	StatusUnknown          AttemptStatus = "unknown"
)

// Attempt records one provider interaction for one email or SMS message.
// Message intent remains channel-specific; provider execution history does not.
type Attempt struct {
	ID                      uuid.UUID
	TeamID                  uuid.UUID
	Channel                 messaging.Channel
	EmailMessageID          *uuid.UUID
	SMSMessageID            *uuid.UUID
	AttemptNumber           int32
	Status                  AttemptStatus
	Provider                string
	ProviderAccount         string
	ProviderMessageID       string
	ProviderStatus          string
	SenderAssetID           *uuid.UUID
	SenderProviderBindingID *uuid.UUID
	ErrorCode               string
	ErrorMessage            string
	ClaimedAt               time.Time
	RequestStartedAt        *time.Time
	RequestCompletedAt      *time.Time
	SubmittedAt             *time.Time
	TerminalAt              *time.Time
	NextReconcileAt         *time.Time
	LastReconciledAt        *time.Time
	ReconcileAttempts       int32
	Metadata                json.RawMessage
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (attempt Attempt) Validate() error {
	if attempt.ID == uuid.Nil || attempt.TeamID == uuid.Nil {
		return errors.New("delivery attempt and team IDs are required")
	}
	if !attempt.Channel.Valid() {
		return errors.New("delivery attempt channel is invalid")
	}
	if err := attempt.validateMessageReference(); err != nil {
		return err
	}
	if attempt.AttemptNumber <= 0 {
		return errors.New("delivery attempt number must be positive")
	}
	if !attempt.Status.Valid() {
		return errors.New("delivery attempt status is invalid")
	}
	if strings.TrimSpace(attempt.ProviderAccount) == "" {
		return errors.New("delivery attempt provider account is required")
	}
	if attempt.Status.RequiresProvider() && strings.TrimSpace(attempt.Provider) == "" {
		return errors.New("delivery attempt provider is required for the current status")
	}
	if attempt.SenderProviderBindingID != nil && attempt.SenderAssetID == nil {
		return errors.New("delivery attempt sender binding requires a sender asset")
	}
	if attempt.ReconcileAttempts < 0 {
		return errors.New("delivery attempt reconciliation count cannot be negative")
	}
	if !validJSONObject(attempt.Metadata) {
		return errors.New("delivery attempt metadata must be a JSON object")
	}
	if attempt.ClaimedAt.IsZero() || attempt.CreatedAt.IsZero() || attempt.UpdatedAt.IsZero() {
		return errors.New("delivery attempt timestamps are required")
	}
	if attempt.RequestStartedAt != nil && attempt.RequestStartedAt.Before(attempt.ClaimedAt) {
		return errors.New("delivery request cannot start before the attempt is claimed")
	}
	if attempt.RequestCompletedAt != nil && attempt.RequestStartedAt != nil &&
		attempt.RequestCompletedAt.Before(*attempt.RequestStartedAt) {
		return errors.New("delivery request cannot complete before it starts")
	}
	if attempt.SubmittedAt != nil && attempt.SubmittedAt.Before(attempt.ClaimedAt) {
		return errors.New("delivery attempt cannot be submitted before it is claimed")
	}
	if attempt.TerminalAt != nil && attempt.TerminalAt.Before(attempt.ClaimedAt) {
		return errors.New("delivery attempt cannot terminate before it is claimed")
	}
	if attempt.Status.Terminal() && attempt.TerminalAt == nil {
		return errors.New("terminal delivery attempt status requires a terminal time")
	}
	return nil
}

func (attempt Attempt) validateMessageReference() error {
	emailSet := attempt.EmailMessageID != nil && *attempt.EmailMessageID != uuid.Nil
	smsSet := attempt.SMSMessageID != nil && *attempt.SMSMessageID != uuid.Nil
	if emailSet == smsSet {
		return errors.New("delivery attempt must reference exactly one message")
	}
	if attempt.Channel == messaging.ChannelEmail && !emailSet {
		return errors.New("email delivery attempt requires an email message")
	}
	if attempt.Channel == messaging.ChannelSMS && !smsSet {
		return errors.New("SMS delivery attempt requires an SMS message")
	}
	return nil
}

func (status AttemptStatus) Valid() bool {
	switch status {
	case StatusClaimed, StatusRequestStarted, StatusSubmissionUnknown,
		StatusSubmitted, StatusAccepted, StatusSent, StatusDelivered,
		StatusRetryableFailure, StatusPermanentFailure, StatusRejected,
		StatusExpired, StatusCanceled, StatusUnknown:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) Terminal() bool {
	switch status {
	case StatusDelivered, StatusPermanentFailure, StatusRejected,
		StatusExpired, StatusCanceled:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) NeedsReconciliation() bool {
	switch status {
	case StatusSubmissionUnknown, StatusSubmitted, StatusAccepted,
		StatusSent, StatusUnknown:
		return true
	default:
		return false
	}
}

func (status AttemptStatus) RequiresProvider() bool {
	switch status {
	case StatusSubmitted, StatusAccepted, StatusSent, StatusDelivered,
		StatusRejected, StatusExpired:
		return true
	default:
		return false
	}
}

// CanTransitionTo defines monotonic provider-attempt lifecycle transitions.
// A retryable failure starts a new attempt rather than reopening this attempt.
func (status AttemptStatus) CanTransitionTo(next AttemptStatus) bool {
	if !status.Valid() || !next.Valid() {
		return false
	}
	if status == next {
		return true
	}
	if status.Terminal() || status == StatusRetryableFailure {
		return false
	}
	allowed := map[AttemptStatus]map[AttemptStatus]struct{}{
		StatusClaimed: {
			StatusRequestStarted: {}, StatusRetryableFailure: {},
			StatusPermanentFailure: {}, StatusCanceled: {},
		},
		StatusRequestStarted: {
			StatusSubmissionUnknown: {}, StatusSubmitted: {},
			StatusRetryableFailure: {}, StatusPermanentFailure: {},
			StatusRejected: {}, StatusCanceled: {},
		},
		StatusSubmissionUnknown: {
			StatusSubmitted: {}, StatusAccepted: {}, StatusSent: {},
			StatusDelivered: {}, StatusRetryableFailure: {},
			StatusPermanentFailure: {}, StatusRejected: {},
			StatusExpired: {}, StatusUnknown: {},
		},
		StatusSubmitted: {
			StatusAccepted: {}, StatusSent: {}, StatusDelivered: {},
			StatusRetryableFailure: {}, StatusPermanentFailure: {},
			StatusRejected: {}, StatusExpired: {}, StatusUnknown: {},
		},
		StatusAccepted: {
			StatusSent: {}, StatusDelivered: {}, StatusRetryableFailure: {},
			StatusPermanentFailure: {}, StatusRejected: {},
			StatusExpired: {}, StatusUnknown: {},
		},
		StatusSent: {
			StatusDelivered: {}, StatusPermanentFailure: {},
			StatusRejected: {}, StatusExpired: {}, StatusUnknown: {},
		},
		StatusUnknown: {
			StatusSubmitted: {}, StatusAccepted: {}, StatusSent: {},
			StatusDelivered: {}, StatusPermanentFailure: {},
			StatusRejected: {}, StatusExpired: {},
		},
	}
	_, ok := allowed[status][next]
	return ok
}

func validJSONObject(value json.RawMessage) bool {
	if len(bytes.TrimSpace(value)) == 0 {
		return true
	}
	if !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil
}
