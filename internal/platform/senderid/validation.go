package senderid

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxNameCharacters = 11

var (
	nameCharactersPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	nameHasLetterPattern  = regexp.MustCompile(`[A-Za-z]`)
)

func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func ValidateName(name string) error {
	name = NormalizeName(name)
	switch {
	case name == "":
		return fmt.Errorf("sender ID name is required")
	case utf8.RuneCountInString(name) > MaxNameCharacters:
		return fmt.Errorf("sender ID name must be at most %d characters", MaxNameCharacters)
	case !nameCharactersPattern.MatchString(name):
		return fmt.Errorf("sender ID name may contain only letters and numbers")
	case !nameHasLetterPattern.MatchString(name):
		return fmt.Errorf("sender ID name must contain at least one letter")
	default:
		return nil
	}
}

func (request CreateRequest) Normalize() CreateRequest {
	request.SenderID = NormalizeName(request.SenderID)
	request.Purpose = strings.TrimSpace(request.Purpose)
	return request
}

func (request CreateRequest) Validate() error {
	return ValidateName(request.SenderID)
}
