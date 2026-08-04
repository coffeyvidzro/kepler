package sms

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

type VerificationSMSInput struct {
	TeamID         uuid.UUID
	VerificationID uuid.UUID
	ChallengeID    uuid.UUID
	Recipient      string
	Sender         string
	Code           string
}

func (s *Service) EnqueueVerificationTx(ctx context.Context, tx pgx.Tx, input VerificationSMSInput) (platformbilling.CommittedAuthorization, error) {
	if s == nil || s.repository == nil || s.delivery == nil || s.billing == nil {
		return platformbilling.CommittedAuthorization{}, errors.New("verification SMS channel is not configured")
	}
	metadata, err := json.Marshal(map[string]string{
		"product": "verify", "verification_id": input.VerificationID.String(), "challenge_id": input.ChallengeID.String(),
	})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	request, err := validateSend(SendRequest{To: input.Recipient, From: input.Sender, Body: "Your Dugble verification code is " + input.Code, Metadata: metadata})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	segments := countSegments(request.Body)
	created, err := s.repository.WithTx(tx).Create(ctx, createMessageParams{
		TeamID: input.TeamID, SenderID: nil, To: request.To, From: request.From, Body: request.Body,
		Status: StatusQueued, Segments: segments, Metadata: request.Metadata, Tags: request.Tags,
		DestinationCountry: request.DestinationCountry,
	})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	messageID, err := uuid.Parse(created.ID)
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	authorization, err := s.billing.AuthorizeSMS(ctx, tx, platformbilling.SMSAuthorizationInput{
		TeamID: input.TeamID, MessageID: messageID, DestinationNumber: request.To, Segments: segments,
	})
	if err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	if err := s.delivery.EnqueueSMSDeliveryTx(ctx, tx, messageID, input.TeamID); err != nil {
		return platformbilling.CommittedAuthorization{}, err
	}
	return platformbilling.CommittedAuthorization{Authorization: authorization, Channel: platformbilling.ChannelSMS, TeamID: input.TeamID, MessageID: messageID}, nil
}

func (s *Service) ObserveVerificationCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	if s != nil && s.billing != nil {
		s.billing.ObserveCommitted(ctx, authorization)
	}
}
