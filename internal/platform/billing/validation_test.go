package billing

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateSMSChargeUsesDefaultCountryProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destination string
		country     string
		provider    string
	}{
		{
			name:        "Ghana uses mnotify",
			destination: "+233201234567",
			country:     "GH",
			provider:    "mnotify",
		},
		{
			name:        "Nigeria uses leamout",
			destination: "+2348012345678",
			country:     "NG",
			provider:    "leamout",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			validated, err := validateSMSCharge(SMSChargeInput{
				TeamID:            uuid.New(),
				MessageID:         uuid.New(),
				DestinationNumber: test.destination,
				Segments:          2,
			})
			if err != nil {
				t.Fatalf("validateSMSCharge() error = %v", err)
			}
			if validated.destinationCountry != test.country {
				t.Fatalf(
					"validateSMSCharge() country = %q, want %q",
					validated.destinationCountry,
					test.country,
				)
			}
			if validated.provider != test.provider {
				t.Fatalf(
					"validateSMSCharge() provider = %q, want %q",
					validated.provider,
					test.provider,
				)
			}
			if validated.routeType != "standard" {
				t.Fatalf("validateSMSCharge() route type = %q, want standard", validated.routeType)
			}
		})
	}
}

func TestValidateSMSChargeRejectsUnsupportedDestination(t *testing.T) {
	t.Parallel()

	_, err := validateSMSCharge(SMSChargeInput{
		TeamID:            uuid.New(),
		MessageID:         uuid.New(),
		DestinationNumber: "+254712345678",
		Segments:          1,
	})
	if !errors.Is(err, ErrInvalidDestination) {
		t.Fatalf("validateSMSCharge() error = %v, want %v", err, ErrInvalidDestination)
	}
}
