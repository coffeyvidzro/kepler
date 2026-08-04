package team

import (
	"net/mail"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func validateTeamName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", apperrors.NewBadRequest("Team name is required")
	}
	return name, nil
}

func validateMarketCode(value string) (string, error) {
	marketCode := strings.ToUpper(strings.TrimSpace(value))
	if marketCode != "GH" && marketCode != "KE" {
		return "", apperrors.NewBadRequest("Market code must be GH or KE")
	}
	return marketCode, nil
}

func validateRequiredTeamField(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", apperrors.NewBadRequest(field + " is required")
	}
	return value, nil
}

func normalizeOptionalTeamField(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func validateTeamID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("Team id must be a valid UUID")
	}
	return id, nil
}

func validateMemberID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("User id must be a valid UUID")
	}
	return id, nil
}

func validateMemberRole(value string) (string, error) {
	role := strings.TrimSpace(value)
	if role != RoleAdmin && role != RoleMember {
		return "", apperrors.NewBadRequest("Role must be admin or member")
	}
	return role, nil
}

func normalizeInvitationEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apperrors.NewBadRequest("A valid invitee email is required")
	}
	return email, nil
}

func validateInvitationRole(value string) (string, error) {
	role := strings.TrimSpace(value)
	if role == "" {
		return RoleMember, nil
	}
	return validateMemberRole(role)
}

func normalizeInvitationToken(value string) string { return strings.TrimSpace(value) }

func validateInvitationToken(value string) (string, error) {
	token := normalizeInvitationToken(value)
	if token == "" {
		return "", apperrors.NewBadRequest("Invitation token is required")
	}
	return token, nil
}
