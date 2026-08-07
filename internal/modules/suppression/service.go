package suppression

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

func (s *Service) Create(ctx context.Context, req CreateRequest) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil { return Suppression{}, err }
	email, err := validateEmail(req.Email)
	if err != nil { return Suppression{}, err }
	value, err := s.repository.Create(ctx, access.Scope.TeamID, email)
	if errors.Is(err, ErrAlreadyExists) { return Suppression{}, apperrors.NewConflict("This email is already suppressed") }
	if err != nil { return Suppression{}, apperrors.NewInternal("Unable to create suppression", err) }
	audit.Record(ctx, access, audit.Event{Action: "suppression.created", ResourceType: "suppression", ResourceID: value.ID})
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsRead)
	if err != nil { return nil, err }
	normalizeListRequest(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil { return nil, apperrors.NewInternal("Unable to list suppressions", err) }
	return values, nil
}

func (s *Service) Get(ctx context.Context, identifier string) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsRead)
	if err != nil { return Suppression{}, err }
	value, err := s.get(ctx, access.Scope.TeamID, identifier)
	if errors.Is(err, pgx.ErrNoRows) { return Suppression{}, apperrors.NewNotFound("Suppression not found") }
	if err != nil { return Suppression{}, apperrors.NewInternal("Unable to get suppression", err) }
	return value, nil
}

func (s *Service) Delete(ctx context.Context, identifier string) (Suppression, error) {
	access, err := requireTenant(ctx, tenant.PermissionSuppressionsWrite)
	if err != nil { return Suppression{}, err }
	var value Suppression
	if id, parseErr := uuid.Parse(strings.TrimSpace(identifier)); parseErr == nil {
		value, err = s.repository.DeleteByID(ctx, id, access.Scope.TeamID)
	} else {
		email, validationErr := validateEmail(identifier)
		if validationErr != nil { return Suppression{}, apperrors.NewBadRequest("Suppression must be a valid UUID or email address") }
		value, err = s.repository.DeleteByEmail(ctx, email, access.Scope.TeamID)
	}
	if errors.Is(err, pgx.ErrNoRows) { return Suppression{}, apperrors.NewNotFound("Suppression not found") }
	if err != nil { return Suppression{}, apperrors.NewInternal("Unable to delete suppression", err) }
	audit.Record(ctx, access, audit.Event{Action: "suppression.deleted", ResourceType: "suppression", ResourceID: value.ID})
	return value, nil
}

func (s *Service) get(ctx context.Context, teamID uuid.UUID, identifier string) (Suppression, error) {
	if id, err := uuid.Parse(strings.TrimSpace(identifier)); err == nil { return s.repository.GetByID(ctx, id, teamID) }
	email, err := validateEmail(identifier)
	if err != nil { return Suppression{}, apperrors.NewBadRequest("Suppression must be a valid UUID or email address") }
	return s.repository.GetByEmail(ctx, email, teamID)
}

func validateEmail(value string) (string, error) {
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || address.Address == "" || address.Name != "" { return "", apperrors.NewBadRequest("Email must be a valid email address") }
	return strings.ToLower(address.Address), nil
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed { return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason) }
	return access, nil
}

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 { req.Limit = 50 }
	if req.Offset < 0 { req.Offset = 0 }
}
