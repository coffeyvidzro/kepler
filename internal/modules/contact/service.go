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

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) Create(ctx context.Context, req CreateRequest) (Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return Contact{}, err
	}
	normalized, err := validateCreate(req)
	if err != nil {
		return Contact{}, err
	}
	value, err := s.repository.Create(ctx, access.Scope.TeamID, normalized)
	if errors.Is(err, ErrAlreadyExists) {
		return Contact{}, apperrors.NewConflict("A contact with this email already exists")
	}
	if errors.Is(err, ErrUnknownProperty) || errors.Is(err, ErrPropertyTypeMismatch) {
		return Contact{}, apperrors.NewBadRequest(err.Error())
	}
	if err != nil {
		return Contact{}, apperrors.NewInternal("Unable to create contact", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact.created", ResourceType: "contact", ResourceID: value.ID})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list contacts", err)
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, value string) (Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsRead)
	if err != nil {
		return Contact{}, err
	}
	id, err := parseID(value, "Contact")
	if err != nil {
		return Contact{}, err
	}
	contact, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return Contact{}, apperrors.NewInternal("Unable to get contact", err)
	}
	return contact, nil
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return Contact{}, err
	}
	id, err := parseID(value, "Contact")
	if err != nil {
		return Contact{}, err
	}
	current, err := s.repository.Get(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return Contact{}, apperrors.NewInternal("Unable to get contact", err)
	}

	merged := CreateRequest{
		Email:        current.Email,
		FirstName:    current.FirstName,
		LastName:     current.LastName,
		Unsubscribed: current.Unsubscribed,
		Properties:   current.Properties,
	}
	if req.Email != nil {
		merged.Email = *req.Email
	}
	if req.FirstName != nil {
		merged.FirstName = normalizeOptional(req.FirstName)
	}
	if req.LastName != nil {
		merged.LastName = normalizeOptional(req.LastName)
	}
	if req.Unsubscribed != nil {
		merged.Unsubscribed = *req.Unsubscribed
	}
	if req.Properties != nil {
		merged.Properties = *req.Properties
	}
	merged, err = validateCreate(merged)
	if err != nil {
		return Contact{}, err
	}
	updated, err := s.repository.Update(ctx, id, access.Scope.TeamID, merged)
	if errors.Is(err, ErrAlreadyExists) {
		return Contact{}, apperrors.NewConflict("A contact with this email already exists")
	}
	if errors.Is(err, ErrUnknownProperty) || errors.Is(err, ErrPropertyTypeMismatch) {
		return Contact{}, apperrors.NewBadRequest(err.Error())
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return Contact{}, apperrors.NewInternal("Unable to update contact", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact.updated", ResourceType: "contact", ResourceID: id.String()})
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, value string) (Contact, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return Contact{}, err
	}
	id, err := parseID(value, "Contact")
	if err != nil {
		return Contact{}, err
	}
	deleted, err := s.repository.Delete(ctx, id, access.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return Contact{}, apperrors.NewInternal("Unable to delete contact", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "contact.deleted", ResourceType: "contact", ResourceID: id.String()})
	return deleted, nil
}

func (s *Service) ListSegments(ctx context.Context, contactValue string) ([]SegmentMembership, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsRead)
	if err != nil {
		return nil, err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return nil, err
	}
	memberships, err := s.repository.ListSegments(ctx, contactID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return nil, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list contact segments", err)
	}
	return memberships, nil
}

func (s *Service) AddSegment(ctx context.Context, contactValue, segmentValue string) (SegmentMembership, bool, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return SegmentMembership{}, false, err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return SegmentMembership{}, false, err
	}
	segmentID, err := parseID(segmentValue, "Segment")
	if err != nil {
		return SegmentMembership{}, false, err
	}
	membership, created, err := s.repository.AddSegment(ctx, contactID, segmentID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return SegmentMembership{}, false, apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrSegmentNotFound) {
		return SegmentMembership{}, false, apperrors.NewNotFound("Segment not found")
	}
	if err != nil {
		return SegmentMembership{}, false, apperrors.NewInternal("Unable to add contact to segment", err)
	}
	if created {
		audit.Record(ctx, access, audit.Event{Action: "contact.segment_added", ResourceType: "contact", ResourceID: contactID.String()})
	}
	return membership, created, nil
}

func (s *Service) RemoveSegment(ctx context.Context, contactValue, segmentValue string) error {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return err
	}
	segmentID, err := parseID(segmentValue, "Segment")
	if err != nil {
		return err
	}
	removed, err := s.repository.RemoveSegment(ctx, contactID, segmentID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrSegmentNotFound) {
		return apperrors.NewNotFound("Segment not found")
	}
	if err != nil {
		return apperrors.NewInternal("Unable to remove contact from segment", err)
	}
	if removed {
		audit.Record(ctx, access, audit.Event{Action: "contact.segment_removed", ResourceType: "contact", ResourceID: contactID.String()})
	}
	return nil
}

func validateCreate(req CreateRequest) (CreateRequest, error) {
	address, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil || address.Address == "" || address.Name != "" {
		return CreateRequest{}, apperrors.NewBadRequest("Email must be a valid email address")
	}
	req.Email = strings.ToLower(address.Address)
	req.FirstName = normalizeOptional(req.FirstName)
	req.LastName = normalizeOptional(req.LastName)
	if req.Properties == nil {
		req.Properties = map[string]any{}
	}
	for key := range req.Properties {
		if strings.TrimSpace(key) == "" {
			return CreateRequest{}, apperrors.NewBadRequest("Contact property keys cannot be empty")
		}
	}
	return req, nil
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

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}

func parseID(value, resource string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest(resource + " id must be a valid UUID")
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
