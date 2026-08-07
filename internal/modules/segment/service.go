package segment

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) Create(ctx context.Context, req CreateRequest) (Segment, error) {
	access, err := requireTenant(ctx, tenant.PermissionSegmentsWrite)
	if err != nil { return Segment{}, err }
	name := strings.TrimSpace(req.Name)
	if name == "" { return Segment{}, apperrors.NewBadRequest("Segment name is required") }
	value, err := s.repository.Create(ctx, access.Scope.TeamID, name)
	if err != nil { return Segment{}, apperrors.NewInternal("Unable to create segment", err) }
	audit.Record(ctx, access, audit.Event{Action: "segment.created", ResourceType: "segment", ResourceID: value.ID})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Segment, error) {
	access, err := requireTenant(ctx, tenant.PermissionSegmentsRead)
	if err != nil { return nil, err }
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil { return nil, apperrors.NewInternal("Unable to list segments", err) }
	return values, nil
}

func (s *Service) Get(ctx context.Context, value string) (Segment, error) {
	access, err := requireTenant(ctx, tenant.PermissionSegmentsRead)
	if err != nil { return Segment{}, err }
	id, err := parseID(value)
	if err != nil { return Segment{}, err }
	segment, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) { return Segment{}, apperrors.NewNotFound("Segment not found") }
	if err != nil { return Segment{}, apperrors.NewInternal("Unable to get segment", err) }
	return segment, nil
}

func (s *Service) ListContacts(ctx context.Context, value string, req ListRequest) ([]Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionSegmentsRead)
	if err != nil { return nil, err }
	id, err := parseID(value)
	if err != nil { return nil, err }
	if _, err := s.repository.Get(ctx, id, access.Scope.TeamID); errors.Is(err, pgx.ErrNoRows) { return nil, apperrors.NewNotFound("Segment not found") } else if err != nil { return nil, apperrors.NewInternal("Unable to get segment", err) }
	normalizeListRequest(&req)
	values, err := s.repository.ListContacts(ctx, id, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil { return nil, apperrors.NewInternal("Unable to list segment contacts", err) }
	return values, nil
}

func (s *Service) Delete(ctx context.Context, value string) (Segment, error) {
	access, err := requireTenant(ctx, tenant.PermissionSegmentsWrite)
	if err != nil { return Segment{}, err }
	id, err := parseID(value)
	if err != nil { return Segment{}, err }
	deleted, err := s.repository.Delete(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) { return Segment{}, apperrors.NewNotFound("Segment not found") }
	if err != nil { return Segment{}, apperrors.NewInternal("Unable to delete segment", err) }
	audit.Record(ctx, access, audit.Event{Action: "segment.deleted", ResourceType: "segment", ResourceID: deleted.ID})
	return deleted, nil
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed { return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason) }
	return access, nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil { return uuid.Nil, apperrors.NewBadRequest("Segment id must be a valid UUID") }
	return id, nil
}

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 { req.Limit = 50 }
	if req.Offset < 0 { req.Offset = 0 }
}
