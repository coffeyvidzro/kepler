package topic

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) CreateAPI(ctx context.Context, request CreateRequest) (MutationResponse, error) {
	value, err := s.Create(ctx, request)
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTopic, ID: value.ID}, nil
}

func (s *Service) ListAPI(ctx context.Context, request APIListRequest) (ListResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionTopicsRead)
	if err != nil {
		return ListResponse{}, err
	}
	if err := normalizeAPIListRequest(&request); err != nil {
		return ListResponse{}, err
	}
	after, err := parseTopicCursor(request.After)
	if err != nil {
		return ListResponse{}, err
	}
	before, err := parseTopicCursor(request.Before)
	if err != nil {
		return ListResponse{}, err
	}
	cursor := after
	if cursor == nil {
		cursor = before
	}
	if cursor != nil {
		exists, lookupErr := s.repository.CursorExists(ctx, access.Scope.TeamID, *cursor)
		if lookupErr != nil {
			return ListResponse{}, apperrors.NewInternal("Unable to validate topic cursor", lookupErr)
		}
		if !exists {
			return ListResponse{}, apperrors.NewNotFound("Topic cursor not found")
		}
	}
	values, err := s.repository.ListPage(ctx, access.Scope.TeamID, request.Limit+1, after, before)
	if err != nil {
		return ListResponse{}, apperrors.NewInternal("Unable to list topics", err)
	}
	hasMore := len(values) > int(request.Limit)
	if hasMore {
		values = values[:request.Limit]
	}
	if before != nil {
		slices.Reverse(values)
	}
	data := make([]Resource, 0, len(values))
	for _, value := range values {
		data = append(data, resourceFromTopic(value))
	}
	return ListResponse{Object: ObjectList, HasMore: hasMore, Data: data}, nil
}

func (s *Service) GetAPI(ctx context.Context, identifier string) (Resource, error) {
	value, err := s.Get(ctx, identifier)
	if err != nil {
		return Resource{}, err
	}
	return resourceFromTopic(value), nil
}

func (s *Service) UpdateAPI(ctx context.Context, identifier string, request UpdateRequest) (MutationResponse, error) {
	value, err := s.Update(ctx, identifier, request)
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTopic, ID: value.ID}, nil
}

func (s *Service) DeleteAPI(ctx context.Context, identifier string) (DeleteResponse, error) {
	value, err := s.Delete(ctx, identifier)
	if err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Object: ObjectTopic, ID: value.ID, Deleted: true}, nil
}

func resourceFromTopic(value Topic) Resource {
	return Resource{
		Object:              ObjectTopic,
		ID:                  value.ID,
		Name:                value.Name,
		Description:         value.Description,
		DefaultSubscription: value.DefaultSubscription,
		Visibility:          value.Visibility,
		CreatedAt:           value.CreatedAt,
	}
}

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

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}
