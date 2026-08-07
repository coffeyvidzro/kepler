package contactproperty

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

var propertyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) Create(ctx context.Context, req CreateRequest) (MutationResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return MutationResponse{}, err
	}
	validated, err := validateCreate(req)
	if err != nil {
		return MutationResponse{}, err
	}
	value, err := s.repository.Create(ctx, access.Scope.TeamID, validated)
	if errors.Is(err, ErrAlreadyExists) {
		return MutationResponse{}, apperrors.NewConflict("A contact property with this key already exists")
	}
	if err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to create contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.created", ResourceType: "contact_property", ResourceID: value.ID})
	return value.MutationResponse(), nil
}

func (s *Service) List(ctx context.Context, req ListRequest) (ListResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesRead)
	if err != nil {
		return ListResponse{}, err
	}
	if err := normalizeListRequest(&req); err != nil {
		return ListResponse{}, err
	}
	values, hasMore, err := s.repository.List(ctx, access.Scope.TeamID, req)
	if errors.Is(err, ErrCursorNotFound) {
		return ListResponse{}, apperrors.NewBadRequest("Contact property cursor is invalid")
	}
	if err != nil {
		return ListResponse{}, apperrors.NewInternal("Unable to list contact properties", err)
	}
	return ListResponse{Object: "list", HasMore: hasMore, Data: ResourceResponses(values)}, nil
}

func (s *Service) Get(ctx context.Context, value string) (ResourceResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesRead)
	if err != nil {
		return ResourceResponse{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return ResourceResponse{}, err
	}
	property, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ResourceResponse{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return ResourceResponse{}, apperrors.NewInternal("Unable to get contact property", err)
	}
	return property.ResourceResponse(), nil
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (MutationResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return MutationResponse{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return MutationResponse{}, err
	}
	current, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MutationResponse{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to get contact property", err)
	}
	if err := validateFallback(current.Type, req.FallbackValue); err != nil {
		return MutationResponse{}, err
	}
	updated, err := s.repository.Update(ctx, id, access.Scope.TeamID, current.Type, req.FallbackValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return MutationResponse{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to update contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.updated", ResourceType: "contact_property", ResourceID: id.String()})
	return updated.MutationResponse(), nil
}

func (s *Service) Delete(ctx context.Context, value string) (DeleteResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return DeleteResponse{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return DeleteResponse{}, err
	}
	deleted, err := s.repository.Delete(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResponse{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return DeleteResponse{}, apperrors.NewInternal("Unable to delete contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.deleted", ResourceType: "contact_property", ResourceID: id.String()})
	return deleted.DeleteResponse(), nil
}

func validateCreate(req CreateRequest) (CreateRequest, error) {
	req.Key = strings.TrimSpace(req.Key)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if len(req.Key) == 0 || len(req.Key) > 50 || !propertyKeyPattern.MatchString(req.Key) {
		return CreateRequest{}, apperrors.NewBadRequest("Contact property key must contain only letters, numbers, and underscores and be at most 50 characters")
	}
	if req.Type != "string" && req.Type != "number" {
		return CreateRequest{}, apperrors.NewBadRequest("Contact property type must be string or number")
	}
	if err := validateFallback(req.Type, req.FallbackValue); err != nil {
		return CreateRequest{}, err
	}
	return req, nil
}

func validateFallback(valueType string, fallback any) error {
	if fallback == nil {
		return nil
	}
	if valueType == "string" {
		if _, ok := fallback.(string); !ok {
			return apperrors.NewBadRequest("Fallback value must be a string")
		}
		return nil
	}
	if _, ok := numericValue(fallback); !ok {
		return apperrors.NewBadRequest("Fallback value must be a number")
	}
	return nil
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("Contact property id must be a valid UUID")
	}
	return id, nil
}

func normalizeListRequest(req *ListRequest) error {
	req.After = strings.TrimSpace(req.After)
	req.Before = strings.TrimSpace(req.Before)
	if req.After != "" && req.Before != "" {
		return apperrors.NewBadRequest("Only one of after or before may be provided")
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 1 || req.Limit > 100 {
		return apperrors.NewBadRequest("limit must be between 1 and 100")
	}
	return nil
}
