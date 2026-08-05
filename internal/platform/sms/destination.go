package sms

import (
	"errors"
	"regexp"
	"strings"
)

const (
	CountryGhana   = "GH"
	CountryKenya   = "KE"
	CountryNigeria = "NG"
)

var (
	ErrInvalidE164            = errors.New("invalid E.164 phone number")
	ErrUnsupportedDestination = errors.New("unsupported SMS destination country")
	e164Pattern               = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)
)

type Destination struct {
	CallingCode string
	CountryCode string
}

var supportedDestinations = []Destination{
	{CallingCode: "+233", CountryCode: CountryGhana},
	{CallingCode: "+234", CountryCode: CountryNigeria},
	{CallingCode: "+254", CountryCode: CountryKenya},
}

func SupportedDestinations() []Destination {
	result := make([]Destination, len(supportedDestinations))
	copy(result, supportedDestinations)
	return result
}

func ResolveDestinationCountry(number string) (string, error) {
	number = strings.TrimSpace(number)
	if !e164Pattern.MatchString(number) {
		return "", ErrInvalidE164
	}
	for _, destination := range supportedDestinations {
		if strings.HasPrefix(number, destination.CallingCode) {
			return destination.CountryCode, nil
		}
	}
	return "", ErrUnsupportedDestination
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

func IsSupportedDestinationCountry(value string) bool {
	value = NormalizeCountryCode(value)
	for _, destination := range supportedDestinations {
		if destination.CountryCode == value {
			return true
		}
	}
	return false
}
