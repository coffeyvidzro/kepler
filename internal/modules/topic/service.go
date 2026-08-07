package topic

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

func (s *Service) Create(ctx context.Context, req CreateRequest) (Topic, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsWrite)
	if err != nil {
		return Topic{}, err
	}
	validated, err := validateCreate(req)
	if err != nil {
		return Topic{}, err
	}
	value, err := s.repository.Create(ctx, access.Scope.TeamID, validated)
	if err != nil {
		return Topic{}, apperrors.NewInternal("Unable to create topic", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "topic.created", ResourceType: "topic", ResourceID: value.ID})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Topic, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list topics", err)
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, value string) (Topic, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsRead)
	if err != nil {
		return Topic{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Topic{}, err
	}
	result, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Topic{}, apperrors.NewNotFound("Topic not found")
	}
	if err != nil {
		return Topic{}, apperrors.NewInternal("Unable to get topic", err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (Topic, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsWrite)
	if err != nil {
		return Topic{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Topic{}, err
	}
	current, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Topic{}, apperrors.NewNotFound("Topic not found")
	}
	if err != nil {
		return Topic{}, apperrors.NewInternal("Unable to get topic", err)
	}
	name := current.Name
	description := current.Description
	visibility := current.Visibility
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		description = normalizeOptional(*req.Description)
	}
	if req.Visibility != nil {
		visibility = strings.ToLower(strings.TrimSpace(*req.Visibility))
	}
	if err := validateNameDescription(name, description); err != nil {
		return Topic{}, err
	}
	if visibility != "public" && visibility != "private" {
		return Topic{}, apperrors.NewBadRequest("Visibility must be public or private")
	}
	result, err := s.repository.Update(ctx, id, access.Scope.TeamID, name, description, visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return Topic{}, apperrors.NewNotFound("Topic not found")
	}
	if err != nil {
		return Topic{}, apperrors.NewInternal("Unable to update topic", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "topic.updated", ResourceType: "topic", ResourceID: result.ID})
	return result, nil
}

func (s *Service) Delete(ctx context.Context, value string) (Topic, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsWrite)
	if err != nil {
		return Topic{}, err
	}
	id, err := parseID(value)
	if err != nil {
		return Topic{}, err
	}
	result, err := s.repository.Delete(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Topic{}, apperrors.NewNotFound("Topic not found")
	}
	if err != nil {
		return Topic{}, apperrors.NewInternal("Unable to delete topic", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "topic.deleted", ResourceType: "topic", ResourceID: result.ID})
	return result, nil
}

func validateCreate(req CreateRequest) (CreateRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = normalizeOptional(req.Description)
	req.DefaultSubscription = strings.ToLower(strings.TrimSpace(req.DefaultSubscription))
	req.Visibility = strings.ToLower(strings.TrimSpace(req.Visibility))
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	if err := validateNameDescription(req.Name, req.Description); err != nil {
		return CreateRequest{}, err
	}
	if req.DefaultSubscription != "opt_in" && req.DefaultSubscription != "opt_out" {
		return CreateRequest{}, apperrors.NewBadRequest("Default subscription must be opt_in or opt_out")
	}
	if req.Visibility != "public" && req.Visibility != "private" {
		return CreateRequest{}, apperrors.NewBadRequest("Visibility must be public or private")
	}
	return req, nil
}

func validateNameDescription(name string, description *string) error {
	if name == "" || len(name) > 50 {
		return apperrors.NewBadRequest("Topic name is required and must be at most 50 characters")
	}
	if description != nil && len(*description) > 200 {
		return apperrors.NewBadRequest("Topic description must be at most 200 characters")
	}
	return nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest("Topic id must be a valid UUID")
	}
	return id, nil
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}
