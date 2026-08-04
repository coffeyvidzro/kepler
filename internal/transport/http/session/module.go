package session

import (
	"strings"

	sessionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/session"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service = sessionmodule.Service

func validateSessionID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", apperrors.NewBadRequest("Session id is required")
	}
	return id, nil
}
