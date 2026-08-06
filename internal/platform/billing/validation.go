package billing

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

var (
	ErrInvalidTeamID         = errors.New("billing team id is required")
	ErrInvalidMessageID      = errors.New("billing message id is required")
	ErrInvalidDestination    = errors.New("billing destination must be a supported E.164 phone number")
	ErrInvalidSegments       = errors.New("billing segments must be greater than zero")
	ErrInvalidRecipientCount = errors.New("billing email recipient count must be greater than zero")
)

func validateSMSCharge(input SMSChargeInput) (SMSChargeInput, error) {
	if input.TeamID == uuid.Nil {
		return SMSChargeInput{}, ErrInvalidTeamID
	}
	if input.MessageID == uuid.Nil {
		return SMSChargeInput{}, ErrInvalidMessageID
	}
	input.DestinationNumber = strings.TrimSpace(input.DestinationNumber)
	destinationCountry, err := platformsms.ResolveDestinationCountry(input.DestinationNumber)
	if err != nil {
		return SMSChargeInput{}, ErrInvalidDestination
	}
	input.destinationCountry = destinationCountry
	if input.Segments <= 0 {
		return SMSChargeInput{}, ErrInvalidSegments
	}
	return input, nil
}

func validateEmailCharge(input EmailChargeInput) error {
	if input.TeamID == uuid.Nil {
		return ErrInvalidTeamID
	}
	if input.MessageID == uuid.Nil {
		return ErrInvalidMessageID
	}
	if input.RecipientCount <= 0 {
		return ErrInvalidRecipientCount
	}
	return nil
}
