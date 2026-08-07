package topic

import (
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxAPITopicPage = 100

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

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
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
