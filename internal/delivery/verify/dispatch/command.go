package dispatch

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

const (
	Subject   = "dugble.job.verify.dispatch.v1"
	namespace = "https://dugble.com/events/verify/dispatch/"
)

type Command struct {
	VerificationID uuid.UUID `json:"verification_id"`
	ChallengeID    uuid.UUID `json:"challenge_id"`
	TeamID         uuid.UUID `json:"team_id"`
	EncryptedCode  []byte    `json:"encrypted_code"`
	SchemaVersion  int       `json:"schema_version"`
}

func EventID(challengeID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(namespace+challengeID.String()))
}

func ValidateCommand(command Command) error {
	if command.VerificationID == uuid.Nil || command.ChallengeID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("verification, challenge, and team ids are required")
	}
	if len(command.EncryptedCode) == 0 {
		return errors.New("encrypted verification code is required")
	}
	if command.SchemaVersion != 1 {
		return errors.New("unsupported verification dispatch schema version")
	}
	return nil
}

func NewEvent(command Command) (outbox.Event, error) {
	if err := ValidateCommand(command); err != nil {
		return outbox.Event{}, err
	}
	eventID := EventID(command.ChallengeID)
	payload, err := json.Marshal(command)
	if err != nil {
		return outbox.Event{}, err
	}
	return outbox.Event{
		ID:            eventID,
		Subject:       Subject,
		AggregateType: "verification_challenge",
		AggregateID:   command.ChallengeID,
		Payload:       payload,
		Headers: map[string]string{
			"Dugble-Event-Type": "verification.dispatch.requested.v1",
		},
	}, nil
}
