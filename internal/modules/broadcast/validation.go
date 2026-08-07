package broadcast

import (
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func parseID(value, label string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest(label + " must be a valid UUID")
	}
	return id, nil
}

func parseOptionalID(value *string, label string) (*uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	id, err := parseID(*value, label)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
