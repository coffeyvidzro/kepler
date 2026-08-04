package phone

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
	ErrUnsupportedDestination = errors.New("unsupported phone destination country")
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
