package broadcast

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (s *Service) Duplicate(ctx context.Context, identifier string, req DuplicateRequest) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsWrite)
	if err != nil {
		return Broadcast{}, err
	}
	id, err := parseID(identifier, "Broadcast id")
	if err != nil {
		return Broadcast{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Broadcast{}, apperrors.NewBadRequest("Broadcast name is required")
	}
	value, err := s.repository.Duplicate(ctx, tc.Scope.TeamID, uuid.MustParse(id.String()), name)
	if errors.Is(err, ErrNotFound) {
		return Broadcast{}, apperrors.NewNotFound("Broadcast not found")
	}
	if err != nil {
		return Broadcast{}, apperrors.NewInternal("Unable to duplicate broadcast", err)
	}
	return value, nil
}
