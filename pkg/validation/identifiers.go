// Package validation provides reusable, transport-independent validation
// helpers.
package validation

import (
	"errors"
	"net/mail"
	"strings"

	googleuuid "github.com/google/uuid"
)

var (
	ErrInvalidUUID   = errors.New("invalid UUID")
	ErrInvalidEmail  = errors.New("invalid email address")
	ErrInvalidDomain = errors.New("invalid domain name")
)

// UUID parses a required UUID value.
func UUID(value string) (googleuuid.UUID, error) {
	return UUIDField("value", value)
}

// UUIDField parses a required UUID and associates failures with field.
func UUIDField(field, value string) (googleuuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return googleuuid.Nil, fieldError(field, ErrRequired)
	}
	id, err := googleuuid.Parse(value)
	if err != nil || id == googleuuid.Nil {
		return googleuuid.Nil, fieldError(field, ErrInvalidUUID)
	}
	return id, nil
}

// OptionalUUID parses a UUID when value is present.
func OptionalUUID(field, value string) (*googleuuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := UUIDField(field, value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// Email normalizes and validates a mailbox address.
func Email(value string) (string, error) {
	return EmailField("email", value)
}

func EmailField(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fieldError(field, ErrRequired)
	}
	address, err := mail.ParseAddress(value)
	if err != nil || strings.TrimSpace(address.Name) != "" {
		return "", fieldError(field, ErrInvalidEmail)
	}
	normalized := strings.ToLower(strings.TrimSpace(address.Address))
	if normalized == "" || !strings.Contains(normalized, "@") {
		return "", fieldError(field, ErrInvalidEmail)
	}
	return normalized, nil
}

func OptionalEmail(field, value string) (*string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	normalized, err := EmailField(field, value)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

// Domain normalizes and validates an ASCII DNS name.
func Domain(value string) (string, error) {
	return DomainField("domain", value)
}

func DomainField(field, value string) (string, error) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if domain == "" {
		return "", fieldError(field, ErrRequired)
	}
	if len(domain) > 253 {
		return "", fieldError(field, ErrInvalidDomain)
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fieldError(field, ErrInvalidDomain)
	}
	for _, label := range labels {
		if !validDNSLabel(label) {
			return "", fieldError(field, ErrInvalidDomain)
		}
	}
	return domain, nil
}

func validDNSLabel(label string) bool {
	if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, character := range label {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '-' {
			return false
		}
	}
	return true
}
