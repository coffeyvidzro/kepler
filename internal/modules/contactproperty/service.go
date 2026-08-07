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

func (s *Service) Create(ctx context.Context, req CreateRequest) (Property, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return Property{}, err
	}
	validated, err := validateCreate(req)
	if err != nil {
		return Property{}, err
	}
	value, err := s.repository.Create(ctx, access.Scope.TeamID, validated)
	if errors.Is(err, ErrAlreadyExists) {
		return Property{}, apperrors.NewConflict("A contact property with this key already exists")
	}
	if err != nil {
		return Property{}, apperrors.NewInternal("Unable to create contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.created", ResourceType: "contact_property", ResourceID: uuid.MustParse(value.ID)})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Property, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list contact properties", err)
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, value string) (Property, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesRead)
	if err != nil {
		return Property{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Property{}, err
	}
	property, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Property{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return Property{}, apperrors.NewInternal("Unable to get contact property", err)
	}
	return property, nil
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (Property, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return Property{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Property{}, err
	}
	current, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Property{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return Property{}, apperrors.NewInternal("Unable to get contact property", err)
	}
	if err := validateFallback(current.Type, req.FallbackValue); err != nil {
		return Property{}, err
	}
	updated, err := s.repository.Update(ctx, id, access.Scope.TeamID, current.Type, req.FallbackValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return Property{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return Property{}, apperrors.NewInternal("Unable to update contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.updated", ResourceType: "contact_property", ResourceID: id})
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, value string) (Property, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactPropertiesWrite)
	if err != nil {
		return Property{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Property{}, err
	}
	deleted, err := s.repository.Delete(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Property{}, apperrors.NewNotFound("Contact property not found")
	}
	if err != nil {
		return Property{}, apperrors.NewInternal("Unable to delete contact property", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact_property.deleted", ResourceType: "contact_property", ResourceID: id})
	return deleted, nil
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

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}
