package verify

import (
	"context"
	"errors"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (service *Service) EnforceCheckAbuse(ctx context.Context, value string, request AbuseContext) error {
	return service.enforceExistingAbuse(ctx, value, tenant.PermissionVerifyCheck, request, true)
}

func (service *Service) EnforceResendAbuse(ctx context.Context, value string, request AbuseContext) error {
	return service.enforceExistingAbuse(ctx, value, tenant.PermissionVerifySend, request, false)
}

func (service *Service) enforceExistingAbuse(
	ctx context.Context,
	value string,
	permission tenant.Permission,
	request AbuseContext,
	check bool,
) error {
	if service == nil || service.repository == nil {
		return apperrors.NewInternal("Verification service is not configured", nil)
	}
	if service.abuse == nil {
		return nil
	}
	access, err := requireTenant(ctx, permission)
	if err != nil {
		return err
	}
	verificationID, err := parseID(value, "Verification")
	if err != nil {
		return err
	}
	current, err := service.repository.GetVerification(ctx, verificationID, access.Scope.TeamID)
	if errors.Is(err, ErrNotFound) {
		return apperrors.NewNotFound("Verification not found")
	}
	if err != nil {
		return apperrors.NewInternal("Unable to apply verification abuse controls", err)
	}
	normalizedRecipient, err := normalizeRecipient(current.Channel, current.Recipient)
	if err != nil {
		return apperrors.NewInternal("Unable to apply verification abuse controls", err)
	}
	if check {
		err = service.abuse.AllowCheck(ctx, access.Scope.TeamID, verificationID, normalizedRecipient, request.IPHash)
	} else {
		err = service.abuse.AllowResend(ctx, access.Scope.TeamID, verificationID, normalizedRecipient, request.IPHash)
	}
	if err == nil {
		return nil
	}
	var appError *apperrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return apperrors.NewInternal("Unable to apply verification abuse controls", err)
}
