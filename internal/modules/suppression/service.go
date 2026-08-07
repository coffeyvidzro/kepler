package suppression

import (
	"context"
	"errors"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) CreateAPI(ctx context.Context, req CreateRequest) (MutationResponse, error) {
	value, err := s.Create(ctx, req)
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectSuppression, ID: value.ID}, nil
}

func (s *Service) BatchAddAPI(ctx context.Context, req BatchAddRequest) (BatchAddResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil {
		return BatchAddResponse{}, err
	}
	emails, err := validateBatchEmails(req.Emails)
	if err != nil {
		return BatchAddResponse{}, err
	}
	values, err := s.repository.CreateBatch(ctx, access.Scope.TeamID, emails)
	if errors.Is(err, ErrAlreadyExists) {
		return BatchAddResponse{}, apperrors.NewConflict("One or more emails are already suppressed")
	}
	if err != nil {
		return BatchAddResponse{}, apperrors.NewInternal("Unable to create suppressions", err)
	}
	data := make([]MutationResponse, 0, len(values))
	for _, value := range values {
		audit.Record(ctx, access, audit.Event{Action: "suppression.created", ResourceType: "suppression", ResourceID: value.ID})
		data = append(data, MutationResponse{Object: ObjectSuppression, ID: value.ID})
	}
	return BatchAddResponse{Data: data}, nil
}

func (s *Service) ListAPI(ctx context.Context, req APIListRequest) (ListResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsRead)
	if err != nil {
		return ListResponse{}, err
	}
	if err := normalizeAPIListRequest(&req); err != nil {
		return ListResponse{}, err
	}
	after, err := parseSuppressionCursor(req.After)
	if err != nil {
		return ListResponse{}, err
	}
	before, err := parseSuppressionCursor(req.Before)
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
			return ListResponse{}, apperrors.NewInternal("Unable to validate suppression cursor", lookupErr)
		}
		if !exists {
			return ListResponse{}, apperrors.NewNotFound("Suppression cursor not found")
		}
	}
	var origin *string
	if req.Origin != "" {
		origin = &req.Origin
	}
	values, err := s.repository.ListPage(ctx, access.Scope.TeamID, req.Limit+1, after, before, origin)
	if err != nil {
		return ListResponse{}, apperrors.NewInternal("Unable to list suppressions", err)
	}
	hasMore := len(values) > int(req.Limit)
	if hasMore {
		values = values[:req.Limit]
	}
	if before != nil {
		slices.Reverse(values)
	}
	data := make([]Resource, 0, len(values))
	for _, value := range values {
		data = append(data, resourceFromSuppression(value))
	}
	return ListResponse{Object: ObjectList, HasMore: hasMore, Data: data}, nil
}

func (s *Service) GetAPI(ctx context.Context, identifier string) (Resource, error) {
	value, err := s.Get(ctx, identifier)
	if err != nil {
		return Resource{}, err
	}
	return resourceFromSuppression(value), nil
}

func (s *Service) DeleteAPI(ctx context.Context, identifier string) (DeleteResponse, error) {
	value, err := s.Delete(ctx, identifier)
	if err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Object: ObjectSuppression, ID: value.ID, Deleted: true}, nil
}

func (s *Service) BatchRemoveAPI(ctx context.Context, req BatchRemoveRequest) (BatchRemoveResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil {
		return BatchRemoveResponse{}, err
	}
	if (len(req.Emails) == 0) == (len(req.IDs) == 0) {
		return BatchRemoveResponse{}, apperrors.NewBadRequest("Provide either emails or ids, but not both")
	}
	var values []Suppression
	if len(req.Emails) > 0 {
		emails, validationErr := validateBatchEmails(req.Emails)
		if validationErr != nil {
			return BatchRemoveResponse{}, validationErr
		}
		values, err = s.repository.DeleteBatchByEmails(ctx, access.Scope.TeamID, emails)
	} else {
		ids, validationErr := validateBatchIDs(req.IDs)
		if validationErr != nil {
			return BatchRemoveResponse{}, validationErr
		}
		values, err = s.repository.DeleteBatchByIDs(ctx, access.Scope.TeamID, ids)
	}
	if err != nil {
		return BatchRemoveResponse{}, apperrors.NewInternal("Unable to delete suppressions", err)
	}
	data := make([]DeleteResponse, 0, len(values))
	for _, value := range values {
		audit.Record(ctx, access, audit.Event{Action: "suppression.deleted", ResourceType: "suppression", ResourceID: value.ID})
		data = append(data, DeleteResponse{Object: ObjectSuppression, ID: value.ID, Deleted: true})
	}
	return BatchRemoveResponse{Data: data}, nil
}

func resourceFromSuppression(value Suppression) Resource {
	return Resource{
		Object:    ObjectSuppression,
		ID:        value.ID,
		Email:     value.Email,
		Origin:    value.Origin,
		SourceID:  value.SourceID,
		CreatedAt: value.CreatedAt,
	}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil {
		return Suppression{}, err
	}
	email, err := validateEmail(req.Email)
	if err != nil {
		return Suppression{}, err
	}
	value, err := s.repository.Create(ctx, access.Scope.TeamID, email)
	if errors.Is(err, ErrAlreadyExists) {
		return Suppression{}, apperrors.NewConflict("This email is already suppressed")
	}
	if err != nil {
		return Suppression{}, apperrors.NewInternal("Unable to create suppression", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "suppression.created", ResourceType: "suppression", ResourceID: value.ID})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list suppressions", err)
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, identifier string) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsRead)
	if err != nil {
		return Suppression{}, err
	}
	value, err := s.get(ctx, access.Scope.TeamID, identifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return Suppression{}, apperrors.NewNotFound("Suppression not found")
	}
	if err != nil {
		return Suppression{}, apperrors.NewInternal("Unable to get suppression", err)
	}
	return value, nil
}

func (s *Service) Delete(ctx context.Context, identifier string) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil {
		return Suppression{}, err
	}
	var value Suppression
	if id, parseErr := uuid.Parse(strings.TrimSpace(identifier)); parseErr == nil {
		value, err = s.repository.DeleteByID(ctx, id, access.Scope.TeamID)
	} else {
		email, validationErr := validateEmail(identifier)
		if validationErr != nil {
			return Suppression{}, apperrors.NewBadRequest("Suppression must be a valid UUID or email address")
		}
		value, err = s.repository.DeleteByEmail(ctx, email, access.Scope.TeamID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Suppression{}, apperrors.NewNotFound("Suppression not found")
	}
	if err != nil {
		return Suppression{}, apperrors.NewInternal("Unable to delete suppression", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "suppression.deleted", ResourceType: "suppression", ResourceID: value.ID})
	return value, nil
}

func (s *Service) get(ctx context.Context, teamID uuid.UUID, identifier string) (Suppression, error) {
	if id, err := uuid.Parse(strings.TrimSpace(identifier)); err == nil {
		return s.repository.GetByID(ctx, id, teamID)
	}
	email, err := validateEmail(identifier)
	if err != nil {
		return Suppression{}, apperrors.NewBadRequest("Suppression must be a valid UUID or email address")
	}
	return s.repository.GetByEmail(ctx, email, teamID)
}

func validateEmail(value string) (string, error) {
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || address.Address == "" || address.Name != "" {
		return "", apperrors.NewBadRequest("Email must be a valid email address")
	}
	return strings.ToLower(address.Address), nil
}

func validateBatchEmails(values []string) ([]string, error) {
	if len(values) < 1 || len(values) > maxBatchSize {
		return nil, apperrors.NewBadRequest("Emails must contain between 1 and 100 addresses")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		email, err := validateEmail(value)
		if err != nil {
			return nil, err
		}
		result = append(result, email)
	}
	return result, nil
}

func validateBatchIDs(values []string) ([]uuid.UUID, error) {
	if len(values) < 1 || len(values) > maxBatchSize {
		return nil, apperrors.NewBadRequest("Ids must contain between 1 and 100 suppression ids")
	}
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, apperrors.NewBadRequest("Suppression ids must be valid UUIDs")
		}
		result = append(result, id)
	}
	return result, nil
}

func normalizeAPIListRequest(req *APIListRequest) error {
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 1 || req.Limit > 100 {
		return apperrors.NewBadRequest("Limit must be between 1 and 100")
	}
	req.After = strings.TrimSpace(req.After)
	req.Before = strings.TrimSpace(req.Before)
	if req.After != "" && req.Before != "" {
		return apperrors.NewBadRequest("After and before cannot be used together")
	}
	req.Origin = strings.ToLower(strings.TrimSpace(req.Origin))
	if req.Origin != "" && req.Origin != "bounce" && req.Origin != "complaint" && req.Origin != "manual" {
		return apperrors.NewBadRequest("Origin must be bounce, complaint, or manual")
	}
	return nil
}

func parseSuppressionCursor(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, apperrors.NewBadRequest("Suppression cursor must be a valid UUID")
	}
	return &id, nil
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
