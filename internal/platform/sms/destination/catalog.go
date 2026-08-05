package destination

import (
	"errors"
	"regexp"
	"strings"
)

const (
	CountryGhana   = "GH"
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

var supported = []Destination{
	{CallingCode: "+233", CountryCode: CountryGhana},
	{CallingCode: "+234", CountryCode: CountryNigeria},
}

func Supported() []Destination {
	result := make([]Destination, len(supported))
	copy(result, supported)
	return result
}

func ResolveCountry(number string) (string, error) {
	number = strings.TrimSpace(number)
	if !e164Pattern.MatchString(number) {
		return "", ErrInvalidE164
	}
	for _, item := range supported {
		if strings.HasPrefix(number, item.CallingCode) {
			return item.CountryCode, nil
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

func IsSupportedCountry(value string) bool {
	value = NormalizeCountryCode(value)
	for _, item := range supported {
		if item.CountryCode == value {
			return true
		}
	}
	return false
}
