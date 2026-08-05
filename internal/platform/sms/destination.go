package sms

import "github.com/coffeyvidzro/dugble/server/internal/platform/sms/destination"

const (
	CountryGhana   = destination.CountryGhana
	CountryNigeria = destination.CountryNigeria
)

var (
	ErrInvalidE164            = destination.ErrInvalidE164
	ErrUnsupportedDestination = destination.ErrUnsupportedDestination
)

type Destination = destination.Destination

func SupportedDestinations() []Destination {
	return destination.Supported()
}

func ResolveDestinationCountry(number string) (string, error) {
	return destination.ResolveCountry(number)
}

func NormalizeCountryCode(value string) string {
	return destination.NormalizeCountryCode(value)
}

func IsCountryCode(value string) bool {
	return destination.IsCountryCode(value)
}

func IsSupportedDestinationCountry(value string) bool {
	return destination.IsSupportedCountry(value)
}
