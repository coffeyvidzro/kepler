package topic

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	ObjectTopic     = "topic"
	ObjectList      = "list"
	maxAPITopicPage = 100
)

type APIListRequest struct {
	Limit  int32
	After  string
	Before string
}

type MutationResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

type DeleteResponse struct {
	Object  string `json:"object"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type Resource struct {
	Object              string    `json:"object"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         *string   `json:"description"`
	DefaultSubscription string    `json:"default_subscription"`
	Visibility          string    `json:"visibility"`
	CreatedAt           time.Time `json:"created_at"`
}

type ListResponse struct {
	Object  string     `json:"object"`
	HasMore bool       `json:"has_more"`
	Data    []Resource `json:"data"`
}

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

func normalizeAPIListRequest(request *APIListRequest) error {
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > maxAPITopicPage {
		return apperrors.NewBadRequest("Limit must be between 1 and 100")
	}
	request.After = strings.TrimSpace(request.After)
	request.Before = strings.TrimSpace(request.Before)
	if request.After != "" && request.Before != "" {
		return apperrors.NewBadRequest("After and before cannot be used together")
	}
	return nil
}

func parseTopicCursor(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, apperrors.NewBadRequest("Topic cursor must be a valid UUID")
	}
	return &id, nil
}
