package contact

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (s *Service) ListTopics(ctx context.Context, identifier string, req ListContactTopicsRequest) (ContactTopicListResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsRead)
	if err != nil {
		return ContactTopicListResponse{}, err
	}
	identifier, err = validateContactIdentifier(identifier)
	if err != nil {
		return ContactTopicListResponse{}, err
	}
	if err := normalizeContactTopicListRequest(&req); err != nil {
		return ContactTopicListResponse{}, err
	}
	topics, hasMore, _, err := s.repository.ListTopics(ctx, identifier, access.Scope.TeamID, req)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContactTopicListResponse{}, apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrContactTopicCursorNotFound) {
		return ContactTopicListResponse{}, apperrors.NewBadRequest("Contact topic cursor is invalid")
	}
	if err != nil {
		return ContactTopicListResponse{}, apperrors.NewInternal("Unable to list contact topics", err)
	}
	return ContactTopicListResponse{Object: ObjectList, HasMore: hasMore, Data: topics}, nil
}

func (s *Service) UpdateTopics(ctx context.Context, identifier string, updates UpdateContactTopicsRequest) (UpdateContactTopicsResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return UpdateContactTopicsResponse{}, err
	}
	identifier, err = validateContactIdentifier(identifier)
	if err != nil {
		return UpdateContactTopicsResponse{}, err
	}
	updates, err = validateContactTopicUpdates(updates)
	if err != nil {
		return UpdateContactTopicsResponse{}, err
	}
	contactID, err := s.repository.UpdateTopics(ctx, identifier, access.Scope.TeamID, updates)
	if errors.Is(err, pgx.ErrNoRows) {
		return UpdateContactTopicsResponse{}, apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrTopicNotFound) {
		return UpdateContactTopicsResponse{}, apperrors.NewNotFound("Topic not found")
	}
	if err != nil {
		return UpdateContactTopicsResponse{}, apperrors.NewInternal("Unable to update contact topics", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact.topics_updated", ResourceType: "contact", ResourceID: contactID})
	return UpdateContactTopicsResponse{ID: contactID}, nil
}

func normalizeContactTopicListRequest(req *ListContactTopicsRequest) error {
	req.After = strings.TrimSpace(req.After)
	req.Before = strings.TrimSpace(req.Before)
	if req.After != "" && req.Before != "" {
		return apperrors.NewBadRequest("Only one of after or before may be provided")
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 1 || req.Limit > maxContactTopicPage {
		return apperrors.NewBadRequest("limit must be between 1 and 100")
	}
	return nil
}

func validateContactTopicUpdates(updates UpdateContactTopicsRequest) (UpdateContactTopicsRequest, error) {
	if len(updates) == 0 {
		return nil, apperrors.NewBadRequest("At least one topic subscription update is required")
	}
	if len(updates) > maxContactTopicPage {
		return nil, apperrors.NewBadRequest("No more than 100 topic subscriptions may be updated at once")
	}
	validated := make(UpdateContactTopicsRequest, len(updates))
	for index, update := range updates {
		id := strings.TrimSpace(update.ID)
		if _, err := uuid.Parse(id); err != nil {
			return nil, apperrors.NewBadRequest("Topic id must be a valid UUID")
		}
		subscription := strings.ToLower(strings.TrimSpace(update.Subscription))
		if subscription != SubscriptionOptIn && subscription != SubscriptionOptOut {
			return nil, apperrors.NewBadRequest("Topic subscription must be opt_in or opt_out")
		}
		validated[index] = UpdateContactTopic{ID: id, Subscription: subscription}
	}
	return validated, nil
}

func validateContactIdentifier(identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", apperrors.NewBadRequest("Contact id or email is required")
	}
	if _, err := uuid.Parse(identifier); err == nil {
		return identifier, nil
	}
	address, err := mail.ParseAddress(identifier)
	if err != nil || address.Name != "" || !strings.EqualFold(address.Address, identifier) {
		return "", apperrors.NewBadRequest("Contact identifier must be a valid UUID or email address")
	}
	return strings.ToLower(address.Address), nil
}
