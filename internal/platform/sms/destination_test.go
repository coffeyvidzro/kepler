package sms_test

import (
	"testing"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

func TestSupportedDestinationCountryUsesResolutionCatalog(t *testing.T) {
	t.Parallel()

	if !platformsms.IsSupportedDestinationCountry("gh") {
		t.Fatal("IsSupportedDestinationCountry(gh) = false")
	}
	if platformsms.IsSupportedDestinationCountry("ZA") {
		t.Fatal("IsSupportedDestinationCountry(ZA) = true")
	}

	destinations := platformsms.SupportedDestinations()
	if len(destinations) == 0 {
		t.Fatal("SupportedDestinations() returned no destinations")
	}
	destinations[0].CountryCode = "XX"
	if !platformsms.IsSupportedDestinationCountry(platformsms.CountryGhana) {
		t.Fatal("SupportedDestinations() exposed mutable internal state")
	}
}
