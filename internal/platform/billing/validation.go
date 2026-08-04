package billing

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/phone"
)

var (
	ErrInvalidTeamID      = errors.New("billing team id is required")
	ErrInvalidMessageID   = errors.New("billing message id is required")
	ErrInvalidDestination = errors.New("billing destination must be a supported E.164 phone number")
	ErrInvalidSegments    = errors.New("billing segments must be greater than zero")
)

func validateSMSAuthorization(input SMSAuthorizationInput) (SMSAuthorizationInput, error) {
	if input.TeamID == uuid.Nil {
		return SMSAuthorizationInput{}, ErrInvalidTeamID
	}
	if input.MessageID == uuid.Nil {
		return SMSAuthorizationInput{}, ErrInvalidMessageID
	}
	input.DestinationNumber = strings.TrimSpace(input.DestinationNumber)
	destinationCountry, err := phone.ResolveDestinationCountry(input.DestinationNumber)
	if err != nil {
		return SMSAuthorizationInput{}, ErrInvalidDestination
	}
	input.destinationCountry = destinationCountry
	input.routeType = "standard"
	switch destinationCountry {
	case "GH":
		input.provider = "mnotify"
	case "KE":
		input.provider = "celcom"
	case "NG":
		input.provider = "arkesel"
	default:
		return SMSAuthorizationInput{}, ErrInvalidDestination
	}
	if input.Segments <= 0 {
		return SMSAuthorizationInput{}, ErrInvalidSegments
	}
	return input, nil
}

func validateEmailAuthorization(input EmailAuthorizationInput) error {
	if input.TeamID == uuid.Nil {
		return ErrInvalidTeamID
	}
	if input.MessageID == uuid.Nil {
		return ErrInvalidMessageID
	}
	return nil
}
