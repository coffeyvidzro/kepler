package channel

import (
	"context"

	"github.com/jackc/pgx/v5"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

type SMS struct {
	service *smsmodule.Service
	sender  string
}

func NewSMS(service *smsmodule.Service, sender string) *SMS {
	return &SMS{service: service, sender: sender}
}

func (adapter *SMS) DispatchTx(ctx context.Context, tx pgx.Tx, input Input) (platformbilling.CommittedAuthorization, error) {
	return adapter.service.EnqueueVerificationTx(ctx, tx, smsmodule.VerificationSMSInput{
		TeamID: input.TeamID, VerificationID: input.VerificationID, ChallengeID: input.ChallengeID,
		Recipient: input.Recipient, Sender: adapter.sender, Code: input.Code,
	})
}

func (adapter *SMS) ObserveCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	adapter.service.ObserveVerificationCommitted(ctx, authorization)
}
