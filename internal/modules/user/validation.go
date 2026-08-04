package user

import (
	"net/mail"
	"strings"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const minimumPasswordLength = 12

func validateID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", apperrors.NewBadRequest("User id is required")
	}
	return id, nil
}

func validateName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", apperrors.NewBadRequest("Name is required")
	}
	return name, nil
}

func validateEmail(value string) (string, error) {
	email := normalizeEmail(value)
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apperrors.NewBadRequest("A valid email is required")
	}
	return email, nil
}

func validatePassword(value string) (string, error) {
	password := strings.TrimSpace(value)
	if len(password) < minimumPasswordLength {
		return "", apperrors.NewBadRequest("Password must be at least 12 characters")
	}
	return password, nil
}

func normalizeEmail(email string) string {
	value := strings.TrimSpace(strings.ToLower(email))
	address, err := mail.ParseAddress(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(strings.ToLower(address.Address))
}

func uniqueEmails(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeEmail(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
