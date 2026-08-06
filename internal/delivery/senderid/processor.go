package senderidreconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

func (consumer *Consumer) process(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim RegistrationClaim,
) error {
	if claim.ProviderSubmittedAt == nil && !strings.EqualFold(claim.ProviderStatus, providerStatusSubmissionUnknown) {
		return consumer.submit(ctx, provider, claim)
	}
	return consumer.checkStatus(ctx, provider, claim)
}

func (consumer *Consumer) submit(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim RegistrationClaim,
) error {
	providerCtx, cancel := context.WithTimeout(ctx, consumer.config.ProviderTimeout)
	response, err := provider.Create(providerCtx, platformsenderid.CreateRequest{SenderID: claim.Name})
	cancel()
	if err != nil {
		providerStatus := providerStatusSubmissionFailed
		if !definitiveProviderError(err) {
			providerStatus = providerStatusSubmissionUnknown
		}
		return consumer.recordFailure(ctx, claim, providerStatus, err)
	}
	if err := validateCreateResponse(provider, claim, response); err != nil {
		return consumer.recordFailure(ctx, claim, providerStatusSubmissionUnknown, err)
	}

	switch response.Status {
	case platformsenderid.StatusPending:
		return consumer.repository.CompleteSubmission(
			ctx,
			claim.ID,
			consumer.workerID,
			response.Status,
			consumer.now().Add(consumer.config.PendingCheckInterval),
		)
	case platformsenderid.StatusApproved, platformsenderid.StatusRejected:
		return consumer.completeStatus(ctx, claim, &platformsenderid.StatusResponse{
			ProviderID:     response.ProviderID,
			SenderID:       response.SenderID,
			Status:         response.Status,
			ProviderStatus: response.Status,
		})
	default:
		return consumer.recordFailure(
			ctx,
			claim,
			providerStatusSubmissionUnknown,
			fmt.Errorf("provider returned unknown Sender ID creation status %q", response.Status),
		)
	}
}

func (consumer *Consumer) checkStatus(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim RegistrationClaim,
) error {
	providerCtx, cancel := context.WithTimeout(ctx, consumer.config.ProviderTimeout)
	response, err := provider.CheckStatus(providerCtx, claim.Name)
	cancel()
	if err != nil {
		providerStatus := claim.ProviderStatus
		if providerStatus == "" {
			providerStatus = platformsenderid.StatusUnknown
		}
		return consumer.recordFailure(ctx, claim, providerStatus, err)
	}
	if err := validateStatusResponse(provider, claim, response); err != nil {
		return consumer.recordFailure(ctx, claim, platformsenderid.StatusUnknown, err)
	}
	return consumer.completeStatus(ctx, claim, response)
}

func (consumer *Consumer) completeStatus(
	ctx context.Context,
	claim RegistrationClaim,
	response *platformsenderid.StatusResponse,
) error {
	var rejectionReason *string
	nextCheckAt := consumer.now()
	switch response.Status {
	case platformsenderid.StatusPending:
		nextCheckAt = nextCheckAt.Add(consumer.config.PendingCheckInterval)
	case platformsenderid.StatusApproved:
	case platformsenderid.StatusRejected:
		reason := "Sender ID was rejected by " + response.ProviderID
		rejectionReason = &reason
	default:
		return consumer.recordFailure(
			ctx,
			claim,
			response.ProviderStatus,
			fmt.Errorf("provider returned unknown Sender ID status %q", response.Status),
		)
	}
	return consumer.repository.CompleteStatus(
		ctx,
		claim.ID,
		consumer.workerID,
		response.Status,
		response.ProviderStatus,
		response.Whitelisted,
		rejectionReason,
		nextCheckAt,
	)
}

func validateCreateResponse(
	provider platformsenderid.Provider,
	claim RegistrationClaim,
	response *platformsenderid.CreateResponse,
) error {
	if response == nil {
		return errors.New("sender ID provider returned an empty creation response")
	}
	if !strings.EqualFold(strings.TrimSpace(response.ProviderID), strings.TrimSpace(provider.ID())) {
		return fmt.Errorf("sender ID provider response ID %q does not match %q", response.ProviderID, provider.ID())
	}
	if !strings.EqualFold(strings.TrimSpace(response.SenderID), strings.TrimSpace(claim.Name)) {
		return fmt.Errorf("sender ID provider response name %q does not match %q", response.SenderID, claim.Name)
	}
	return nil
}

func validateStatusResponse(
	provider platformsenderid.Provider,
	claim RegistrationClaim,
	response *platformsenderid.StatusResponse,
) error {
	if response == nil {
		return errors.New("sender ID provider returned an empty status response")
	}
	if !strings.EqualFold(strings.TrimSpace(response.ProviderID), strings.TrimSpace(provider.ID())) {
		return fmt.Errorf("sender ID provider response ID %q does not match %q", response.ProviderID, provider.ID())
	}
	if !strings.EqualFold(strings.TrimSpace(response.SenderID), strings.TrimSpace(claim.Name)) {
		return fmt.Errorf("sender ID provider response name %q does not match %q", response.SenderID, claim.Name)
	}
	return nil
}
