package senderid

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	maxSenderIDLength = 11
	maxPurposeLength  = 500
	maxProviderLength = 120
)

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context) ([]SenderID, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionSenderIDsRead)
	if err != nil {
		return nil, err
	}
	senderIDs, err := s.repository.List(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list sender IDs", err)
	}
	return senderIDs, nil
}

func (s *Service) Get(ctx context.Context, senderID string) (SenderID, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionSenderIDsRead)
	if err != nil {
		return SenderID{}, err
	}
	parsedSenderID, err := uuid.Parse(strings.TrimSpace(senderID))
	if err != nil {
		return SenderID{}, apperrors.NewBadRequest("Sender ID id must be a valid UUID")
	}
	value, err := s.repository.Get(ctx, parsedSenderID, tenantContext.Scope.TeamID)
	if err != nil {
		return SenderID{}, apperrors.NewNotFound("Sender ID not found")
	}
	return value, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (SenderID, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionSenderIDsCreate)
	if err != nil {
		return SenderID{}, err
	}
	name, countryCode, purpose, provider, err := validateCreate(req)
	if err != nil {
		return SenderID{}, err
	}
	senderID, err := s.repository.Create(
		ctx,
		tenantContext.Scope.TeamID,
		name,
		countryCode,
		purpose,
		provider,
		tenantContext.Actor.UserID,
	)
	if err != nil {
		if errors.Is(err, ErrSenderIDAlreadyExists) {
			return SenderID{}, apperrors.NewConflict(
				"Sender ID already exists for this team and country",
			)
		}
		return SenderID{}, apperrors.NewInternal("Unable to create sender ID", err)
	}
	return senderID, nil
}

func (s *Service) Delete(ctx context.Context, senderID string) (SenderID, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionSenderIDsDelete)
	if err != nil {
		return SenderID{}, err
	}
	parsedSenderID, err := uuid.Parse(strings.TrimSpace(senderID))
	if err != nil {
		return SenderID{}, apperrors.NewBadRequest("Sender ID id must be a valid UUID")
	}
	value, err := s.repository.Delete(ctx, parsedSenderID, tenantContext.Scope.TeamID)
	if err != nil {
		return SenderID{}, apperrors.NewNotFound("Sender ID not found")
	}
	return value, nil
}

func validateCreate(req CreateRequest) (string, string, string, *string, error) {
	name := strings.TrimSpace(req.Name)
	countryCode := strings.ToUpper(strings.TrimSpace(req.CountryCode))
	purpose := strings.TrimSpace(req.Purpose)
	provider := normalizeOptional(req.Provider)

	if name == "" {
		return "", "", "", nil, apperrors.NewBadRequest("Sender ID name is required")
	}
	if len(name) > maxSenderIDLength {
		return "", "", "", nil, apperrors.NewBadRequest("Sender ID name must be at most 11 characters")
	}
	if !countryCodePattern.MatchString(countryCode) {
		return "", "", "", nil, apperrors.NewBadRequest("Country code must be a valid ISO 3166-1 alpha-2 code")
	}
	if purpose == "" {
		return "", "", "", nil, apperrors.NewBadRequest("Sender ID purpose is required")
	}
	if len(purpose) > maxPurposeLength {
		return "", "", "", nil, apperrors.NewBadRequest("Sender ID purpose must be at most 500 characters")
	}
	if provider != nil && len(*provider) > maxProviderLength {
		return "", "", "", nil, apperrors.NewBadRequest("Sender ID provider must be at most 120 characters")
	}
	return name, countryCode, purpose, provider, nil
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

func requireTenantPermission(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	tenantContext, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return tenantContext, nil
}
