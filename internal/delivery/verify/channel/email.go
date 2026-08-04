package channel

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

type Input struct {
	TeamID         uuid.UUID
	VerificationID uuid.UUID
	ChallengeID    uuid.UUID
	Recipient      string
	Code           string
}

type Email struct{ service *emailmodule.Service }

func NewEmail(service *emailmodule.Service) *Email { return &Email{service: service} }

func (adapter *Email) DispatchTx(ctx context.Context, tx pgx.Tx, input Input) (platformbilling.CommittedAuthorization, error) {
	return adapter.service.EnqueueVerificationTx(ctx, tx, emailmodule.VerificationEmailInput{
		TeamID: input.TeamID, VerificationID: input.VerificationID, ChallengeID: input.ChallengeID,
		Recipient: input.Recipient, Code: input.Code,
	})
}

func (adapter *Email) ObserveCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	adapter.service.ObserveVerificationCommitted(ctx, authorization)
}
