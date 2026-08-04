package email

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type VerificationEmailInput struct {
	TeamID         uuid.UUID
	VerificationID uuid.UUID
	ChallengeID    uuid.UUID
	Recipient      string
	Code           string
}

func (s *Service) EnqueueVerificationTx(ctx context.Context, tx pgx.Tx, input VerificationEmailInput) (platformbilling.CommittedAuthorization, error) {
	if s == nil || s.repository == nil || s.delivery == nil || s.billing == nil {
		return platformbilling.CommittedAuthorization{}, errors.New("verification email channel is not configured")
	}
	metadata, err := json.Marshal(map[string]string{
		"product": "verify", "verification_id": input.VerificationID.String(), "challenge_id": input.ChallengeID.String(),
	})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	request := SendRequest{
		Stream:   MessageTypeTransactional,
		To:       EmailAddressList{{Email: input.Recipient}},
		Subject:  "Your verification code",
		Text:     "Your Dugble verification code is " + input.Code + ".",
		HTML:     "<p>Your Dugble verification code is <strong>" + input.Code + "</strong>.</p>",
		Metadata: metadata,
	}
	validated, err := validateSend(request, s.config)
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	if s.config.DefaultProvider == "" || s.config.DefaultRegion == "" {
		return platformbilling.CommittedAuthorization{}, errors.New("verification email route is not configured")
	}
	validated.Provider = s.config.DefaultProvider
	validated.ProviderRegion = s.config.DefaultRegion
	validated.DeliveryRoute = platformemail.SystemDeliveryRoute()
	validated.SenderDomainID = nil
	message, err := s.repository.CreateTx(ctx, tx, input.TeamID, validated)
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	messageID, err := uuid.Parse(message.ID)
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	authorization, err := s.billing.AuthorizeEmail(ctx, tx, platformbilling.EmailAuthorizationInput{TeamID: input.TeamID, MessageID: messageID})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	if err := enqueueDelivery(ctx, s.delivery, tx, messageID, input.TeamID, nil); err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	return platformbilling.CommittedAuthorization{Authorization: authorization, Channel: platformbilling.ChannelEmail, TeamID: input.TeamID, MessageID: messageID}, nil
}

func (s *Service) ObserveVerificationCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	if s != nil && s.billing != nil {
		s.billing.ObserveCommitted(ctx, authorization)
	}
}
