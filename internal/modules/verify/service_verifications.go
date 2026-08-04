package verify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func (service *Service) Create(ctx context.Context, req CreateVerificationRequest) (Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifySend)
	if err != nil {
		return Verification{}, err
	}
	if err := service.requireRuntime(); err != nil {
		return Verification{}, err
	}
	verification, err := database.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (Verification, error) {
		repository := service.repository.WithTx(tx)
		configured, resolveErr := service.resolveService(ctx, repository, access.Scope.TeamID, req)
		if resolveErr != nil {
			return Verification{}, resolveErr
		}
		if !configured.Enabled {
			return Verification{}, apperrors.NewConflict("Verification service is disabled")
		}
		validated, validationErr := validateCreateVerification(req, configured)
		if validationErr != nil {
			return Verification{}, validationErr
		}
		serviceID := uuid.MustParse(configured.ID)
		if service.abuse != nil {
			if abuseErr := service.abuse.AllowCreate(
				ctx,
				access.Scope.TeamID,
				serviceID,
				validated.RecipientNormalized,
				req.IPHash,
			); abuseErr != nil {
				return Verification{}, abuseErr
			}
		}
		now := service.now().UTC()
		expiresAt := now.Add(time.Duration(configured.TTLSeconds) * time.Second)
		created, createErr := repository.CreateVerification(
			ctx, access.Scope.TeamID, serviceID, validated,
			pgtype.Timestamptz{Time: expiresAt, Valid: true},
		)
		if createErr != nil {
			return Verification{}, createErr
		}
		verificationID := uuid.MustParse(created.ID)
		challenge, codeErr := service.codes.Generate(access.Scope.TeamID, verificationID, 1, configured.CodeLength)
		if codeErr != nil {
			return Verification{}, codeErr
		}
		createdChallenge, challengeErr := repository.CreateChallenge(
			ctx, access.Scope.TeamID, verificationID, 1, challenge.CodeHMAC, validated.Channel,
			pgtype.Timestamptz{Time: expiresAt, Valid: true},
		)
		if challengeErr != nil {
			return Verification{}, challengeErr
		}
		if queueErr := service.dispatch.EnqueueVerificationDispatchTx(ctx, tx, verifydispatch.Command{
			VerificationID: verificationID,
			ChallengeID:    createdChallenge.ID,
			TeamID:         access.Scope.TeamID,
			EncryptedCode:  challenge.SealedCode,
			SchemaVersion:  1,
		}); queueErr != nil {
			return Verification{}, fmt.Errorf("enqueue verification dispatch: %w", queueErr)
		}
		if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationCreated, created); eventErr != nil {
			return Verification{}, eventErr
		}
		return created, nil
	})
	if errors.Is(err, ErrNotFound) {
		return Verification{}, apperrors.NewNotFound("Verification service not found")
	}
	if err != nil {
		var appError *apperrors.AppError
		if errors.As(err, &appError) {
			return Verification{}, appError
		}
		return Verification{}, apperrors.NewInternal("Unable to create verification", err)
	}
	return verification, nil
}

func (service *Service) List(ctx context.Context, req ListRequest) ([]Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	result, err := service.repository.ListVerifications(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list verifications", err)
	}
	return result, nil
}

func (service *Service) Get(ctx context.Context, value string) (Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyRead)
	if err != nil {
		return Verification{}, err
	}
	id, err := parseID(value, "Verification")
	if err != nil {
		return Verification{}, err
	}
	result, err := service.repository.GetVerification(ctx, id, access.Scope.TeamID)
	if errors.Is(err, ErrNotFound) {
		return Verification{}, apperrors.NewNotFound("Verification not found")
	}
	if err != nil {
		return Verification{}, apperrors.NewInternal("Unable to get verification", err)
	}
	return result, nil
}
