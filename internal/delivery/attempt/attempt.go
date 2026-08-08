package attempt

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Attempt records one provider interaction for one email or SMS message.
// Message intent remains channel-specific; provider execution history does not.
type Attempt struct {
	ID                 uuid.UUID
	TeamID             uuid.UUID
	Channel            Channel
	EmailMessageID     *uuid.UUID
	SMSMessageID       *uuid.UUID
	AttemptNumber      int32
	Status             AttemptStatus
	Provider           string
	ProviderAccount    string
	ProviderMessageID  string
	ProviderStatus     string
	SenderDomainID     *uuid.UUID
	SenderID           *uuid.UUID
	ErrorCode          string
	ErrorMessage       string
	ClaimedAt          time.Time
	RequestStartedAt   *time.Time
	RequestCompletedAt *time.Time
	SubmittedAt        *time.Time
	TerminalAt         *time.Time
	NextReconcileAt    *time.Time
	LastReconciledAt   *time.Time
	ReconcileAttempts  int32
	Metadata           json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (attempt Attempt) MessageReference() MessageReference {
	return MessageReference{
		Channel:        attempt.Channel,
		EmailMessageID: attempt.EmailMessageID,
		SMSMessageID:   attempt.SMSMessageID,
	}
}

func (attempt Attempt) ProviderRoute() ProviderRoute {
	return ProviderRoute{
		Provider:        attempt.Provider,
		ProviderAccount: attempt.ProviderAccount,
		SenderDomainID:  attempt.SenderDomainID,
		SenderID:        attempt.SenderID,
	}
}

func (attempt Attempt) Validate() error {
	if attempt.ID == uuid.Nil || attempt.TeamID == uuid.Nil {
		return errors.New("delivery attempt and team IDs are required")
	}
	if err := attempt.MessageReference().Validate(); err != nil {
		return err
	}
	if attempt.AttemptNumber <= 0 {
		return errors.New("delivery attempt number must be positive")
	}
	if !attempt.Status.Valid() {
		return errors.New("delivery attempt status is invalid")
	}
	if err := attempt.ProviderRoute().Validate(attempt.Status.RequiresProvider()); err != nil {
		return err
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
