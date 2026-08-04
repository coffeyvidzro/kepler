package verify

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (service *Service) CreateService(ctx context.Context, req CreateServiceRequest) (VerificationService, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyManage)
	if err != nil {
		return VerificationService{}, err
	}
	value, err := validateCreateService(req)
	if err != nil {
		return VerificationService{}, err
	}
	value.Metadata, err = normalizeJSONObject(value.Metadata)
	if err != nil {
		return VerificationService{}, err
	}
	created, err := service.repository.CreateService(ctx, access.Scope.TeamID, value)
	if errors.Is(err, ErrDuplicateKey) {
		return VerificationService{}, apperrors.NewConflict("Verification service key already exists")
	}
	if err != nil {
		return VerificationService{}, apperrors.NewInternal("Unable to create verification service", err)
	}
	return created, nil
}

func (service *Service) ListServices(ctx context.Context, req ListRequest) ([]VerificationService, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	result, err := service.repository.ListServices(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list verification services", err)
	}
	return result, nil
}

func (service *Service) GetService(ctx context.Context, value string) (VerificationService, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyRead)
	if err != nil {
		return VerificationService{}, err
	}
	id, err := parseID(value, "Verification service")
	if err != nil {
		return VerificationService{}, err
	}
	result, err := service.repository.GetService(ctx, id, access.Scope.TeamID)
	if errors.Is(err, ErrNotFound) {
		return VerificationService{}, apperrors.NewNotFound("Verification service not found")
	}
	if err != nil {
		return VerificationService{}, apperrors.NewInternal("Unable to get verification service", err)
	}
	return result, nil
}

func (service *Service) UpdateService(ctx context.Context, value string, req UpdateServiceRequest) (VerificationService, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyManage)
	if err != nil {
		return VerificationService{}, err
	}
	id, err := parseID(value, "Verification service")
	if err != nil {
		return VerificationService{}, err
	}
	result, err := database.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (VerificationService, error) {
		repository := service.repository.WithTx(tx)
		current, getErr := repository.GetService(ctx, id, access.Scope.TeamID)
		if getErr != nil {
			return VerificationService{}, getErr
		}
		validated, validationErr := validateUpdateService(current, req)
		if validationErr != nil {
			return VerificationService{}, validationErr
		}
		validated.Metadata, validationErr = normalizeJSONObject(validated.Metadata)
		if validationErr != nil {
			return VerificationService{}, validationErr
		}
		return repository.UpdateService(ctx, id, access.Scope.TeamID, validated)
	})
	if errors.Is(err, ErrNotFound) {
		return VerificationService{}, apperrors.NewNotFound("Verification service not found")
	}
	if err != nil {
		var appError *apperrors.AppError
		if errors.As(err, &appError) {
			return VerificationService{}, appError
		}
		return VerificationService{}, apperrors.NewInternal("Unable to update verification service", err)
	}
	return result, nil
}
