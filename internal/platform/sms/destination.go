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

type destinationPrefix struct {
	prefix      string
	countryCode string
}

var destinationPrefixes = []destinationPrefix{
	{prefix: "+233", countryCode: CountryGhana},
	{prefix: "+234", countryCode: CountryNigeria},
	{prefix: "+254", countryCode: CountryKenya},
}

func ResolveDestinationCountry(number string) (string, error) {
	number = strings.TrimSpace(number)
	if !e164Pattern.MatchString(number) {
		return "", ErrInvalidE164
	}
	for _, candidate := range destinationPrefixes {
		if strings.HasPrefix(number, candidate.prefix) {
			return candidate.countryCode, nil
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
