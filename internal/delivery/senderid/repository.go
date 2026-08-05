package senderidreconciliation

import (
	"context"
	"time"

	"github.com/google/uuid"

	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
)

type repository interface {
	ClaimPendingRegistrations(context.Context, string, string, int32, time.Time) ([]senderidmodule.RegistrationClaim, error)
	CompleteSubmission(context.Context, uuid.UUID, string, string, time.Time) error
	CompleteStatus(context.Context, uuid.UUID, string, string, string, bool, *string, time.Time) error
	RecordProviderFailure(context.Context, uuid.UUID, string, string, error, time.Time) error
}
