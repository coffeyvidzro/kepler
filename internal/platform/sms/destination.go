package sms

import (
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/platform/phone"
)

const (
	CountryGhana   = phone.CountryGhana
	CountryKenya   = phone.CountryKenya
	CountryNigeria = phone.CountryNigeria
)

var ErrUnsupportedDestination = phone.ErrUnsupportedDestination

func ResolveDestinationCountry(number string) (string, error) {
	return phone.ResolveDestinationCountry(number)
}

func NormalizeCountryCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func IsCountryCode(value string) bool {
	value = NormalizeCountryCode(value)
	if len(value) != 2 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}
