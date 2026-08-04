package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (service *Service) resolveService(ctx context.Context, repository *Repository, teamID uuid.UUID, req CreateVerificationRequest) (VerificationService, error) {
	if strings.TrimSpace(req.ServiceID) != "" {
		id, err := parseID(req.ServiceID, "Verification service")
		if err != nil {
			return VerificationService{}, err
		}
		return repository.GetService(ctx, id, teamID)
	}
	key := strings.ToLower(strings.TrimSpace(req.Service))
	if key == "" {
		return VerificationService{}, apperrors.NewBadRequest("Exactly one of service_id or service is required")
	}
	return repository.GetServiceByKey(ctx, teamID, key)
}

func (service *Service) emit(ctx context.Context, tx pgx.Tx, eventType platformevent.Type, verification Verification) error {
	data, err := json.Marshal(verification)
	if err != nil {
		return fmt.Errorf("encode verification event: %w", err)
	}
	verificationID, err := uuid.Parse(verification.ID)
	if err != nil {
		return fmt.Errorf("parse verification event id: %w", err)
	}
	teamID, err := uuid.Parse(verification.TeamID)
	if err != nil {
		return fmt.Errorf("parse verification event team id: %w", err)
	}
	_, err = service.events.EmitTx(ctx, tx, platformevent.Envelope{
		Type: eventType, TeamID: teamID, ObjectType: "verification", ObjectID: &verificationID,
		Data: data, OccurredAt: verification.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("emit verification event: %w", err)
	}
	return nil
}

func (service *Service) requireRuntime() error {
	if service == nil || service.repository == nil {
		return apperrors.NewInternal("Verification repository is not configured", nil)
	}
	if service.codes == nil {
		return apperrors.NewInternal("Verification code manager is not configured", nil)
	}
	if service.dispatch == nil {
		return apperrors.NewInternal("Verification dispatch queue is not configured", nil)
	}
	if service.events == nil {
		return apperrors.NewInternal("Verification events are not configured", nil)
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
