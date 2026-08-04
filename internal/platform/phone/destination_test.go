package phone

import (
	"errors"
	"testing"
)

func TestResolveDestinationCountry(t *testing.T) {
	for _, test := range []struct {
		number string
		want   string
	}{
		{number: "+233241234567", want: CountryGhana},
		{number: "+2348012345678", want: CountryNigeria},
		{number: "+254712345678", want: CountryKenya},
	} {
		got, err := ResolveDestinationCountry(test.number)
		if err != nil || got != test.want {
			t.Fatalf("ResolveDestinationCountry(%q) = %q, %v; want %q", test.number, got, err, test.want)
		}
	}
}

func TestResolveDestinationCountryRejectsInvalidAndUnsupportedNumbers(t *testing.T) {
	if _, err := ResolveDestinationCountry("0241234567"); !errors.Is(err, ErrInvalidE164) {
		t.Fatalf("local number error = %v, want ErrInvalidE164", err)
	}
	if _, err := ResolveDestinationCountry("+12025550123"); !errors.Is(err, ErrUnsupportedDestination) {
		t.Fatalf("unsupported number error = %v, want ErrUnsupportedDestination", err)
	}
}
