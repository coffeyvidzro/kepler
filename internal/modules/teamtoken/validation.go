package teamtoken

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func validateTokenID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("Token id must be a valid UUID")
	}
	return id, nil
}

func validateMutation(name string, permissions []string, expiresAt *time.Time) (string, []string, *time.Time, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, nil, apperrors.NewBadRequest("Token name is required")
	}
	if len(name) > maxNameLength {
		return "", nil, nil, apperrors.NewBadRequest("Token name is too long")
	}
	expiresAt, err := normalizeExpiresAt(expiresAt)
	if err != nil {
		return "", nil, nil, err
	}
	validated, err := validatePermissions(permissions)
	if err != nil {
		return "", nil, nil, err
	}
	return name, validated, expiresAt, nil
}

func normalizeExpiresAt(expiresAt *time.Time) (*time.Time, error) {
	now := time.Now().UTC()
	if expiresAt == nil {
		value := now.Add(defaultTokenTTL)
		return &value, nil
	}
	value := expiresAt.UTC()
	if !value.After(now) {
		return nil, apperrors.NewBadRequest("Token expiration must be in the future")
	}
	if value.After(now.Add(maxTokenTTL)) {
		return nil, apperrors.NewBadRequest("Token expiration cannot be more than 365 days in the future")
	}
	return &value, nil
}

func validatePermissions(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	permissions := make([]string, 0, len(values))
	for _, value := range values {
		permission := tenant.Permission(strings.TrimSpace(value))
		if permission == "" {
			continue
		}
		if !IsAllowedPermission(permission) {
			return nil, apperrors.NewBadRequest("Unsupported token permission")
		}
		key := string(permission)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		permissions = append(permissions, key)
	}
	if len(permissions) == 0 {
		return nil, apperrors.NewBadRequest("At least one token permission is required")
	}
	return permissions, nil
}

func IsAllowedPermission(permission tenant.Permission) bool {
	_, ok := allowedPermissions[permission]
	return ok
}
