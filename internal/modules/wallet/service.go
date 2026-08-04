package wallet

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type store interface {
	Get(context.Context, uuid.UUID) (Wallet, error)
	ListLedger(context.Context, uuid.UUID, int32, int32) ([]LedgerEntry, error)
	Credit(context.Context, uuid.UUID, int64, string) (Wallet, error)
}

type Service struct {
	repository store
}

func NewService(repository store) *Service {
	return &Service{repository: repository}
}

func (s *Service) Get(ctx context.Context) (Wallet, error) {
	access, decision := tenant.ResolveAccess(ctx, tenant.PermissionWalletRead)
	if !decision.Allowed {
		return Wallet{}, apperrors.NewForbidden(decision.Reason)
	}
	wallet, err := s.repository.Get(ctx, access.Scope.TeamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Wallet{}, apperrors.NewNotFound("Wallet not found")
		}
		return Wallet{}, apperrors.NewInternal("Unable to get wallet", err)
	}
	return wallet, nil
}

func (s *Service) ListLedger(ctx context.Context, limit int32, offset int32) (LedgerPage, error) {
	access, decision := tenant.ResolveAccess(ctx, tenant.PermissionWalletLedgerRead)
	if !decision.Allowed {
		return LedgerPage{}, apperrors.NewForbidden(decision.Reason)
	}
	limit, offset, err := validateLedgerPage(limit, offset)
	if err != nil {
		return LedgerPage{}, err
	}
	entries, err := s.repository.ListLedger(ctx, access.Scope.TeamID, limit, offset)
	if err != nil {
		return LedgerPage{}, apperrors.NewInternal("Unable to list wallet ledger", err)
	}
	return LedgerPage{Entries: entries, Limit: limit, Offset: offset}, nil
}

// Credit is intentionally not exposed by the public wallet routes. A future
// payment provider integration should call it only after verifying a payment.
func (s *Service) Credit(ctx context.Context, input CreditInput) (Wallet, error) {
	teamID, amountUnits, referenceID, err := validateCredit(input)
	if err != nil {
		return Wallet{}, err
	}
	wallet, err := s.repository.Credit(ctx, teamID, amountUnits, referenceID)
	if err != nil {
		return Wallet{}, apperrors.NewInternal("Unable to credit wallet", err)
	}
	return wallet, nil
}
