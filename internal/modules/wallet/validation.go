package wallet

import (
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	defaultLedgerLimit int32 = 50
	maxLedgerLimit     int32 = 100
)

func validateCredit(input CreditInput) (uuid.UUID, int64, string, error) {
	teamID, err := uuid.Parse(strings.TrimSpace(input.TeamID))
	if err != nil {
		return uuid.Nil, 0, "", apperrors.NewBadRequest("Team id must be a valid UUID")
	}
	if input.AmountUnits <= 0 {
		return uuid.Nil, 0, "", apperrors.NewBadRequest("Credit amount must be greater than zero")
	}
	referenceID := strings.TrimSpace(input.ReferenceID)
	if referenceID == "" {
		return uuid.Nil, 0, "", apperrors.NewBadRequest("Credit reference is required")
	}
	return teamID, input.AmountUnits, referenceID, nil
}

func validateLedgerPage(limit int32, offset int32) (int32, int32, error) {
	if limit == 0 {
		limit = defaultLedgerLimit
	}
	if limit < 0 || limit > maxLedgerLimit {
		return 0, 0, apperrors.NewBadRequest("Ledger limit must be between 1 and 100")
	}
	if offset < 0 {
		return 0, 0, apperrors.NewBadRequest("Ledger offset cannot be negative")
	}
	return limit, offset, nil
}
