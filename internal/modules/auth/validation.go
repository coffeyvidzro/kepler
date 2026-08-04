package auth

import (
	"net/mail"
	"strings"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const minimumPasswordLength = 12

func validateCredentials(emailValue, nameValue, passwordValue string) (string, string, string, error) {
	email, err := validateEmail(emailValue)
	if err != nil {
		return "", "", "", err
	}
	name := strings.TrimSpace(nameValue)
	if name == "" {
		return "", "", "", apperrors.NewBadRequest("Name is required")
	}
	password, err := validatePassword(passwordValue)
	if err != nil {
		return "", "", "", err
	}
	return email, name, password, nil
}

func validateLoginRequest(request LoginRequest) (string, string, error) {
	email := normalizeEmail(request.Email)
	password := strings.TrimSpace(request.Password)
	if email == "" || password == "" {
		return "", "", apperrors.NewBadRequest("Email and password are required")
	}
	return email, password, nil
}

func validateEmailToken(emailValue, tokenValue string) (string, string, error) {
	email := normalizeEmail(emailValue)
	token := strings.TrimSpace(tokenValue)
	if email == "" || token == "" {
		return "", "", apperrors.NewBadRequest("Email and token are required")
	}
	return email, token, nil
}

func validateResetPasswordRequest(request ResetPasswordRequest) (string, string, string, error) {
	email, token, err := validateEmailToken(request.Email, request.Token)
	if err != nil {
		return "", "", "", err
	}
	password, err := validatePassword(request.Password)
	if err != nil {
		return "", "", "", err
	}
	return email, token, password, nil
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

func emailVerificationIdentifier(email string) string { return "email.verify:" + normalizeEmail(email) }
func passwordResetIdentifier(email string) string     { return "password.reset:" + normalizeEmail(email) }
