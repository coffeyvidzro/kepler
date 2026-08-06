package argus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	argusdispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/argus/dispatch"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type dispatchQueue interface {
	EnqueueVerificationDispatchTx(context.Context, pgx.Tx, argusdispatch.Command) error
}

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
}

type Service struct {
	repository *Repository
	codes      *CodeManager
	dispatch   dispatchQueue
	events     eventEmitter
	now        func() time.Time
}

func NewService(repository *Repository, codes *CodeManager, dispatch dispatchQueue, events eventEmitter) *Service {
	return &Service{repository: repository, codes: codes, dispatch: dispatch, events: events, now: time.Now}
}

func (service *Service) Create(ctx context.Context, req CreateVerificationRequest) (Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifySend)
	if err != nil {
		return Verification{}, err
	}
	if err := service.requireRuntime(); err != nil {
		return Verification{}, err
	}
	validated, err := validateCreateVerification(req)
	if err != nil {
		return Verification{}, err
	}
	verification, err := postgres.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (Verification, error) {
		repository := service.repository.WithTx(tx)
		now := service.now().UTC()
		expiresAt := now.Add(time.Duration(validated.TTLSeconds) * time.Second)
		created, createErr := repository.CreateVerification(
			ctx, access.Scope.TeamID, validated,
			pgtype.Timestamptz{Time: expiresAt, Valid: true},
		)
		if createErr != nil {
			return Verification{}, createErr
		}
		verificationID := uuid.MustParse(created.ID)
		challenge, codeErr := service.codes.Generate(access.Scope.TeamID, verificationID, 1, validated.CodeLength)
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
		if queueErr := service.dispatch.EnqueueVerificationDispatchTx(ctx, tx, argusdispatch.Command{
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

func (service *Service) Check(ctx context.Context, value string, req CheckRequest) (CheckResponse, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifyCheck)
	if err != nil {
		return CheckResponse{}, err
	}
	if err := service.requireRuntime(); err != nil {
		return CheckResponse{}, err
	}
	id, err := parseID(value, "Verification")
	if err != nil {
		return CheckResponse{}, err
	}
	validated, err := validateCheck(req)
	if err != nil {
		return CheckResponse{}, err
	}
	response, err := postgres.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (CheckResponse, error) {
		repository := service.repository.WithTx(tx)
		locked, lockErr := repository.GetVerificationForUpdate(ctx, id, access.Scope.TeamID)
		if lockErr != nil {
			return CheckResponse{}, lockErr
		}
		current := verificationFromSQLC(locked)
		if current.Status != StatusPending {
			return terminalCheckResponse(current), nil
		}
		challenge, challengeErr := repository.GetActiveChallengeForUpdate(ctx, id, access.Scope.TeamID)
		if challengeErr != nil {
			return CheckResponse{}, challengeErr
		}
		now := service.now().UTC()
		if !current.ExpiresAt.After(now) || !challenge.ExpiresAt.Time.After(now) {
			if expireErr := repository.MarkChallengeExpired(ctx, challenge.ID, access.Scope.TeamID); expireErr != nil {
				return CheckResponse{}, expireErr
			}
			expired, expireErr := repository.MarkExpired(ctx, id, access.Scope.TeamID)
			if expireErr != nil {
				return CheckResponse{}, expireErr
			}
			if attemptErr := repository.RecordAttempt(ctx, id, challenge.ID, access.Scope.TeamID, StatusExpired, validated); attemptErr != nil {
				return CheckResponse{}, attemptErr
			}
			if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationExpired, expired); eventErr != nil {
				return CheckResponse{}, eventErr
			}
			return CheckResponse{ID: expired.ID, Status: expired.Status, Expired: true}, nil
		}
		incremented, incrementErr := repository.IncrementAttempt(ctx, id, access.Scope.TeamID)
		if incrementErr != nil {
			return CheckResponse{}, incrementErr
		}
		if service.codes.Matches(access.Scope.TeamID, id, challenge.Sequence, validated.Code, challenge.CodeHmac) {
			if attemptErr := repository.RecordAttempt(ctx, id, challenge.ID, access.Scope.TeamID, StatusApproved, validated); attemptErr != nil {
				return CheckResponse{}, attemptErr
			}
			if supersedeErr := repository.SupersedeChallenges(ctx, id, access.Scope.TeamID); supersedeErr != nil {
				return CheckResponse{}, supersedeErr
			}
			approved, approveErr := repository.Approve(ctx, id, access.Scope.TeamID)
			if approveErr != nil {
				return CheckResponse{}, approveErr
			}
			if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationApproved, approved); eventErr != nil {
				return CheckResponse{}, eventErr
			}
			return CheckResponse{ID: approved.ID, Status: approved.Status, Valid: true}, nil
		}
		if incremented.AttemptCount >= current.MaxAttempts {
			if attemptErr := repository.RecordAttempt(ctx, id, challenge.ID, access.Scope.TeamID, StatusMaxAttemptsReached, validated); attemptErr != nil {
				return CheckResponse{}, attemptErr
			}
			if supersedeErr := repository.SupersedeChallenges(ctx, id, access.Scope.TeamID); supersedeErr != nil {
				return CheckResponse{}, supersedeErr
			}
			failed, failErr := repository.MarkMaxAttempts(ctx, id, access.Scope.TeamID)
			if failErr != nil {
				return CheckResponse{}, failErr
			}
			if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationMaxAttemptsReached, failed); eventErr != nil {
				return CheckResponse{}, eventErr
			}
			return CheckResponse{ID: failed.ID, Status: failed.Status}, nil
		}
		if attemptErr := repository.RecordAttempt(ctx, id, challenge.ID, access.Scope.TeamID, "incorrect", validated); attemptErr != nil {
			return CheckResponse{}, attemptErr
		}
		updated := verificationFromSQLC(incremented)
		if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationIncorrect, updated); eventErr != nil {
			return CheckResponse{}, eventErr
		}
		return CheckResponse{ID: updated.ID, Status: updated.Status}, nil
	})
	if errors.Is(err, ErrNotFound) {
		return CheckResponse{}, apperrors.NewNotFound("Verification not found")
	}
	if err != nil {
		var appError *apperrors.AppError
		if errors.As(err, &appError) {
			return CheckResponse{}, appError
		}
		return CheckResponse{}, apperrors.NewInternal("Unable to check verification", err)
	}
	return response, nil
}

func (service *Service) Resend(ctx context.Context, value string) (Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifySend)
	if err != nil {
		return Verification{}, err
	}
	if err := service.requireRuntime(); err != nil {
		return Verification{}, err
	}
	id, err := parseID(value, "Verification")
	if err != nil {
		return Verification{}, err
	}
	result, err := postgres.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (Verification, error) {
		repository := service.repository.WithTx(tx)
		locked, lockErr := repository.GetVerificationForUpdate(ctx, id, access.Scope.TeamID)
		if lockErr != nil {
			return Verification{}, lockErr
		}
		current := verificationFromSQLC(locked)
		now := service.now().UTC()
		if policyErr := validateResendVerification(current, now); policyErr != nil {
			return Verification{}, policyErr
		}
		challenge, challengeErr := repository.GetActiveChallengeForUpdate(ctx, id, access.Scope.TeamID)
		if challengeErr != nil {
			return Verification{}, challengeErr
		}
		if policyErr := validateResendChallenge(
			challenge.CreatedAt.Time,
			challenge.ExpiresAt.Time,
			current.ResendCooldownSeconds,
			now,
		); policyErr != nil {
			return Verification{}, policyErr
		}
		sequence := challenge.Sequence + 1
		expiresAt := now.Add(time.Duration(current.TTLSeconds) * time.Second)
		generated, codeErr := service.codes.Generate(access.Scope.TeamID, id, sequence, current.CodeLength)
		if codeErr != nil {
			return Verification{}, codeErr
		}
		if supersedeErr := repository.SupersedeChallenges(ctx, id, access.Scope.TeamID); supersedeErr != nil {
			return Verification{}, supersedeErr
		}
		incremented, incrementErr := repository.IncrementResend(ctx, id, access.Scope.TeamID, pgtype.Timestamptz{Time: expiresAt, Valid: true})
		if incrementErr != nil {
			return Verification{}, incrementErr
		}
		createdChallenge, createErr := repository.CreateChallenge(
			ctx, access.Scope.TeamID, id, sequence, generated.CodeHMAC, current.Channel,
			pgtype.Timestamptz{Time: expiresAt, Valid: true},
		)
		if createErr != nil {
			return Verification{}, createErr
		}
		if queueErr := service.dispatch.EnqueueVerificationDispatchTx(ctx, tx, argusdispatch.Command{
			VerificationID: id, ChallengeID: createdChallenge.ID, TeamID: access.Scope.TeamID,
			EncryptedCode: generated.SealedCode, SchemaVersion: 1,
		}); queueErr != nil {
			return Verification{}, fmt.Errorf("enqueue verification resend: %w", queueErr)
		}
		updated := verificationFromSQLC(incremented)
		if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationResent, updated); eventErr != nil {
			return Verification{}, eventErr
		}
		return updated, nil
	})
	if errors.Is(err, ErrNotFound) {
		return Verification{}, apperrors.NewNotFound("Verification not found")
	}
	if err != nil {
		var appError *apperrors.AppError
		if errors.As(err, &appError) {
			return Verification{}, appError
		}
		return Verification{}, apperrors.NewInternal("Unable to resend verification", err)
	}
	return result, nil
}

func (service *Service) Cancel(ctx context.Context, value string) (Verification, error) {
	access, err := requireTenant(ctx, tenant.PermissionVerifySend)
	if err != nil {
		return Verification{}, err
	}
	if service.events == nil {
		return Verification{}, apperrors.NewInternal("Verification events are not configured", nil)
	}
	id, err := parseID(value, "Verification")
	if err != nil {
		return Verification{}, err
	}
	result, err := postgres.InTransactionResult(ctx, service.repository.db, func(tx pgx.Tx) (Verification, error) {
		repository := service.repository.WithTx(tx)
		locked, lockErr := repository.GetVerificationForUpdate(ctx, id, access.Scope.TeamID)
		if lockErr != nil {
			return Verification{}, lockErr
		}
		current := verificationFromSQLC(locked)
		if current.Status == StatusCanceled {
			return current, nil
		}
		if current.Status != StatusPending {
			return Verification{}, apperrors.NewConflict("Only pending verifications can be canceled")
		}
		if supersedeErr := repository.SupersedeChallenges(ctx, id, access.Scope.TeamID); supersedeErr != nil {
			return Verification{}, supersedeErr
		}
		canceled, cancelErr := repository.Cancel(ctx, id, access.Scope.TeamID)
		if cancelErr != nil {
			return Verification{}, cancelErr
		}
		if eventErr := service.emit(ctx, tx, platformevent.TypeVerificationCanceled, canceled); eventErr != nil {
			return Verification{}, eventErr
		}
		return canceled, nil
	})
	if errors.Is(err, ErrNotFound) {
		return Verification{}, apperrors.NewNotFound("Verification not found")
	}
	if err != nil {
		var appError *apperrors.AppError
		if errors.As(err, &appError) {
			return Verification{}, appError
		}
		return Verification{}, apperrors.NewInternal("Unable to cancel verification", err)
	}
	return result, nil
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
