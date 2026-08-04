package session

import (
	"strings"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func validateSessionID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", apperrors.NewBadRequest("Session id is required")
	}
	return id, nil
}
