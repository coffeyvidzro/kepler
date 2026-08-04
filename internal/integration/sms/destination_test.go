package sms

import (
	"errors"
	"testing"
)

func TestResolveDestinationCountry(t *testing.T) {
	tests := []struct {
		number string
		want   string
	}{
		{number: "+233241234567", want: CountryGhana},
		{number: "+2348012345678", want: CountryNigeria},
		{number: "+254712345678", want: CountryKenya},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			got, err := ResolveDestinationCountry(test.number)
			if err != nil {
				t.Fatalf("ResolveDestinationCountry returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("ResolveDestinationCountry(%q) = %q, want %q", test.number, got, test.want)
			}
		})
	}
}

func TestResolveDestinationCountryRejectsUnsupportedMarket(t *testing.T) {
	_, err := ResolveDestinationCountry("+12025550123")
	if !errors.Is(err, ErrUnsupportedDestination) {
		t.Fatalf("ResolveDestinationCountry error = %v, want ErrUnsupportedDestination", err)
	}
}

func TestSendRequestNormalizeResolvesDestinationCountry(t *testing.T) {
	req := (SendRequest{To: " +233241234567 ", From: " DUGBLE ", Message: "hello"}).Normalize()
	if req.DestinationCountry != CountryGhana {
		t.Fatalf("DestinationCountry = %q, want %q", req.DestinationCountry, CountryGhana)
	}
}

func TestSendRequestValidateRejectsMismatchedCountry(t *testing.T) {
	err := (SendRequest{
		To:                 "+233241234567",
		From:               "DUGBLE",
		Message:            "hello",
		DestinationCountry: CountryNigeria,
	}).Validate()
	if err == nil {
		t.Fatal("Validate returned nil error for mismatched destination country")
	}
}
