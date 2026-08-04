package sms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Sender interface {
	Send(ctx context.Context, req smsapi.SendRequest) (*smsapi.SendResponse, error)
	CheckStatus(ctx context.Context, providerID string, providerMessageID string) (*smsapi.StatusResponse, error)
}

type scheduledDeliveryQueue interface {
	EnqueueSMSDeliveryAtTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) error
	CancelSMSDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error
	RescheduleSMSDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) error
}

// DeliveryQueue enqueues durable SMS delivery work.
//
// Implementations should persist jobs durably before returning.
type DeliveryQueue interface {
	EnqueueSMSDelivery(ctx context.Context, messageID uuid.UUID, teamID uuid.UUID) error
	EnqueueSMSDeliveryTx(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, teamID uuid.UUID) error
}

type Service struct {
	repository *Repository
	sender     Sender
	delivery   DeliveryQueue
	billing    platformbilling.SMSBilling
}

func NewService(repository *Repository, sender Sender, delivery DeliveryQueue, billing platformbilling.SMSBilling) *Service {
	return &Service{repository: repository, sender: sender, delivery: delivery, billing: billing}
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Message, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSRead)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	messages, err := s.repository.List(ctx, tenantContext.Scope.TeamID, limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list SMS messages", err)
	}
	return messages, nil
}

func (s *Service) Get(ctx context.Context, messageID string) (Message, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSRead)
	if err != nil {
		return Message{}, err
	}
	parsedID, err := uuid.Parse(strings.TrimSpace(messageID))
	if err != nil {
		return Message{}, apperrors.NewBadRequest("SMS message id must be a valid UUID")
	}
	message, err := s.repository.Get(ctx, parsedID, tenantContext.Scope.TeamID)
	if err != nil {
		if errors.Is(err, ErrMessageNotFound) {
			return Message{}, apperrors.NewNotFound("SMS message not found")
		}
		return Message{}, apperrors.NewInternal("Unable to get SMS message", err)
	}
	return message, nil
}

func (s *Service) Send(ctx context.Context, req SendRequest) (Message, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSSend)
	if err != nil {
		return Message{}, err
	}
	if s.delivery == nil {
		return Message{}, apperrors.NewInternal("SMS delivery queue is not configured", nil)
	}
	if s.billing == nil {
		return Message{}, apperrors.NewInternal("SMS billing authorization is not configured", nil)
	}

	normalized, err := validateSend(req)
	if err != nil {
		return Message{}, err
	}
	senderID, err := s.repository.FindApprovedSender(ctx, tenantContext.Scope.TeamID, normalized.From)
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to validate SMS sender ID", err)
	}
	if senderID == nil {
		return Message{}, apperrors.NewBadRequest("SMS sender ID must be approved before use")
	}

	segments := countSegments(normalized.Body)
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to begin SMS send transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepository := s.repository.WithTx(tx)

	created, err := txRepository.Create(ctx, createMessageParams{
		TeamID: tenantContext.Scope.TeamID, SenderID: senderID, To: normalized.To, From: normalized.From,
		Body: normalized.Body, Status: StatusQueued, Segments: segments,
		Metadata:           normalized.Metadata,
		Tags:               normalized.Tags,
		ScheduledAt:        normalizedScheduledAt(normalized.ScheduledAt),
		DestinationCountry: normalized.DestinationCountry,
	})
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to create SMS message", err)
	}
	if created.ProviderMessageID != nil || created.Status != StatusQueued {
		return Message{}, apperrors.NewInternal("New SMS message is not in a billable queued state", nil)
	}

	messageID := uuid.MustParse(created.ID)
	authorization, err := s.billing.AuthorizeSMS(ctx, tx, platformbilling.SMSAuthorizationInput{
		TeamID: tenantContext.Scope.TeamID, MessageID: messageID,
		DestinationNumber: normalized.To, Segments: segments,
	})
	if err != nil {
		return Message{}, smsBillingError(err)
	}
	if err := enqueueSMSDelivery(ctx, s.delivery, tx, messageID, tenantContext.Scope.TeamID, created.ScheduledAt); err != nil {
		return Message{}, apperrors.NewInternal("Unable to enqueue SMS delivery", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, apperrors.NewInternal("Unable to commit SMS send transaction", err)
	}
	s.billing.ObserveCommitted(ctx, platformbilling.CommittedAuthorization{
		Authorization: authorization, Channel: platformbilling.ChannelSMS,
		TeamID: tenantContext.Scope.TeamID, MessageID: messageID,
	})

	return created, nil
}

func (s *Service) BatchSend(ctx context.Context, req BatchSendRequest) ([]Message, error) {
	if err := validateBatchSend(req); err != nil {
		return nil, err
	}
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSSend)
	if err != nil {
		return nil, err
	}
	if s.delivery == nil {
		return nil, apperrors.NewInternal("SMS delivery queue is not configured", nil)
	}
	if s.billing == nil {
		return nil, apperrors.NewInternal("SMS billing authorization is not configured", nil)
	}

	type preparedMessage struct {
		request  SendRequest
		senderID *uuid.UUID
		segments int32
	}
	prepared := make([]preparedMessage, len(req.Messages))
	senders := make(map[string]*uuid.UUID)
	for index, request := range req.Messages {
		normalized, err := validateSend(request)
		if err != nil {
			return nil, err
		}
		senderKey := strings.ToLower(normalized.From)
		senderID, exists := senders[senderKey]
		if !exists {
			senderID, err = s.repository.FindApprovedSender(ctx, tenantContext.Scope.TeamID, normalized.From)
			if err != nil {
				return nil, apperrors.NewInternal("Unable to validate SMS sender ID", err)
			}
			if senderID == nil {
				return nil, apperrors.NewBadRequest("SMS sender ID must be approved before use")
			}
			senders[senderKey] = senderID
		}
		prepared[index] = preparedMessage{request: normalized, senderID: senderID, segments: countSegments(normalized.Body)}
	}

	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to begin SMS batch transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txRepository := s.repository.WithTx(tx)
	result := make([]Message, 0, len(prepared))
	committedAuthorizations := make([]platformbilling.CommittedAuthorization, 0, len(prepared))
	for _, item := range prepared {

		created, err := txRepository.Create(ctx, createMessageParams{
			TeamID: tenantContext.Scope.TeamID, SenderID: item.senderID, To: item.request.To, From: item.request.From,
			Body: item.request.Body, Status: StatusQueued, Segments: item.segments,
			Metadata: item.request.Metadata, Tags: item.request.Tags,
			ScheduledAt:        normalizedScheduledAt(item.request.ScheduledAt),
			DestinationCountry: item.request.DestinationCountry,
		})
		if err != nil {
			return nil, apperrors.NewInternal("Unable to create SMS batch message", err)
		}
		messageID := uuid.MustParse(created.ID)
		authorization, err := s.billing.AuthorizeSMS(ctx, tx, platformbilling.SMSAuthorizationInput{
			TeamID: tenantContext.Scope.TeamID, MessageID: messageID,
			DestinationNumber: item.request.To, Segments: item.segments,
		})
		if err != nil {
			return nil, smsBillingError(err)
		}
		if err := enqueueSMSDelivery(ctx, s.delivery, tx, messageID, tenantContext.Scope.TeamID, created.ScheduledAt); err != nil {
			return nil, apperrors.NewInternal("Unable to enqueue SMS batch delivery", err)
		}
		result = append(result, created)
		committedAuthorizations = append(committedAuthorizations, platformbilling.CommittedAuthorization{
			Authorization: authorization, Channel: platformbilling.ChannelSMS,
			TeamID: tenantContext.Scope.TeamID, MessageID: messageID,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.NewInternal("Unable to commit SMS batch transaction", err)
	}
	for _, authorization := range committedAuthorizations {
		s.billing.ObserveCommitted(ctx, authorization)
	}
	return result, nil
}

func smsBillingError(err error) error {
	switch {
	case errors.Is(err, platformbilling.ErrInsufficientBalance):
		return apperrors.NewPaymentRequired("Insufficient wallet balance")
	case errors.Is(err, platformbilling.ErrInvalidDestination):
		return apperrors.NewBadRequest("SMS recipient must be a supported E.164 phone number")
	case errors.Is(err, platformbilling.ErrInvalidSegments):
		return apperrors.NewBadRequest("SMS segment count is invalid")
	case errors.Is(err, platformbilling.ErrTeamNotFound):
		return apperrors.NewNotFound("Billing team not found")
	case errors.Is(err, platformbilling.ErrTeamInactive):
		return apperrors.NewConflict("Team is not active for billing")
	case errors.Is(err, platformbilling.ErrUnsupportedMarket):
		return apperrors.NewConflict("Team market is not supported for billing")
	case errors.Is(err, platformbilling.ErrWalletNotFound):
		return apperrors.NewConflict("Team wallet is not initialized")
	case errors.Is(err, platformbilling.ErrRateNotFound):
		return apperrors.NewServiceUnavailable("SMS pricing is unavailable", err)
	case errors.Is(err, platformbilling.ErrCurrencyMismatch):
		return apperrors.NewConflict("Wallet currency does not match the team market")
	case errors.Is(err, platformbilling.ErrAmountOverflow):
		return apperrors.NewInternal("SMS charge amount exceeds the supported range", err)
	default:
		return apperrors.NewInternal("Unable to authorize SMS billing", err)
	}
}

func (s *Service) Cancel(ctx context.Context, value string) (SendResponse, error) {
	tenantContext, id, queue, err := s.scheduledMutationContext(ctx, value)
	if err != nil {
		return SendResponse{}, err
	}
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to begin SMS cancellation transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	message, err := s.repository.CancelTx(ctx, tx, id, tenantContext.Scope.TeamID)
	if err != nil {
		return SendResponse{}, scheduledMutationError(err, "canceled")
	}
	if err := queue.CancelSMSDeliveryTx(ctx, tx, id, tenantContext.Scope.TeamID); err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to cancel SMS delivery", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to commit SMS cancellation", err)
	}
	return message.SendResponse(), nil
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (SendResponse, error) {
	tenantContext, id, queue, err := s.scheduledMutationContext(ctx, value)
	if err != nil {
		return SendResponse{}, err
	}
	scheduledAt, err := normalizeSMSSchedule(req.ScheduledAt, false)
	if err != nil {
		return SendResponse{}, err
	}
	if scheduledAt == nil {
		return SendResponse{}, apperrors.NewBadRequest("scheduled_at is required")
	}
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to begin SMS update transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	message, err := s.repository.RescheduleTx(ctx, tx, id, tenantContext.Scope.TeamID, *scheduledAt)
	if err != nil {
		return SendResponse{}, scheduledMutationError(err, "updated")
	}
	if err := queue.RescheduleSMSDeliveryTx(ctx, tx, id, tenantContext.Scope.TeamID, *scheduledAt); err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to reschedule SMS delivery", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SendResponse{}, apperrors.NewInternal("Unable to commit SMS update", err)
	}
	return message.SendResponse(), nil
}

func (s *Service) scheduledMutationContext(ctx context.Context, value string) (tenant.AccessContext, uuid.UUID, scheduledDeliveryQueue, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSSend)
	if err != nil {
		return tenant.AccessContext{}, uuid.Nil, nil, err
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return tenant.AccessContext{}, uuid.Nil, nil, apperrors.NewBadRequest("SMS message id must be a valid UUID")
	}
	queue, ok := s.delivery.(scheduledDeliveryQueue)
	if !ok {
		return tenant.AccessContext{}, uuid.Nil, nil, apperrors.NewInternal("SMS delivery queue does not support scheduling", nil)
	}
	return tenantContext, id, queue, nil
}

func scheduledMutationError(err error, action string) error {
	if errors.Is(err, ErrMessageNotFound) {
		return apperrors.NewNotFound("SMS message not found")
	}
	if errors.Is(err, ErrMessageNotSchedulable) {
		return apperrors.NewConflict("Only pending scheduled SMS messages outside the delivery cutoff can be " + action)
	}
	return apperrors.NewInternal("Unable to update SMS message", err)
}

func normalizedScheduledAt(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return &parsed
}

func enqueueSMSDelivery(ctx context.Context, queue DeliveryQueue, tx pgx.Tx, messageID, teamID uuid.UUID, scheduledAt *time.Time) error {
	if scheduledAt == nil {
		return queue.EnqueueSMSDeliveryTx(ctx, tx, messageID, teamID)
	}
	scheduled, ok := queue.(scheduledDeliveryQueue)
	if !ok {
		return errors.New("SMS delivery queue does not support scheduled delivery")
	}
	return scheduled.EnqueueSMSDeliveryAtTx(ctx, tx, messageID, teamID, *scheduledAt)
}

func (s *Service) SyncStatus(ctx context.Context, messageID string) (Message, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionSMSSend)
	if err != nil {
		return Message{}, err
	}
	parsedID, err := uuid.Parse(strings.TrimSpace(messageID))
	if err != nil {
		return Message{}, apperrors.NewBadRequest("SMS message id must be a valid UUID")
	}
	message, err := s.repository.Get(ctx, parsedID, tenantContext.Scope.TeamID)
	if err != nil {
		if errors.Is(err, ErrMessageNotFound) {
			return Message{}, apperrors.NewNotFound("SMS message not found")
		}
		return Message{}, apperrors.NewInternal("Unable to get SMS message", err)
	}
	if message.ProviderID == nil || message.ProviderMessageID == nil {
		return Message{}, apperrors.NewBadRequest("SMS message has not been submitted to a provider")
	}
	if s.sender == nil {
		return Message{}, apperrors.NewInternal("SMS sender is not configured", nil)
	}
	status, err := s.sender.CheckStatus(ctx, *message.ProviderID, *message.ProviderMessageID)
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to sync SMS status", err)
	}
	nextStatus := resolveProviderStatus(message.Status, status.Status)
	if nextStatus == message.Status {
		return message, nil
	}
	updated, err := s.repository.UpdateStatus(ctx, parsedID, tenantContext.Scope.TeamID, nextStatus)
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to update SMS status", err)
	}
	return updated, nil
}

func resolveProviderStatus(current string, providerStatus string) string {
	current = strings.ToLower(strings.TrimSpace(current))
	next := MapProviderStatus(providerStatus)
	if next == StatusUnknown || isTerminalStatus(current) {
		return current
	}

	currentRank, currentIsProgress := statusProgressRank(current)
	nextRank, nextIsProgress := statusProgressRank(next)
	if currentIsProgress && nextIsProgress && nextRank < currentRank {
		return current
	}
	return next
}

func isTerminalStatus(status string) bool {
	switch status {
	case StatusDelivered, StatusUndelivered, StatusRejected, StatusFailed, StatusExpired, StatusCanceled:
		return true
	default:
		return false
	}
}

func statusProgressRank(status string) (int, bool) {
	switch status {
	case StatusQueued:
		return 0, true
	case StatusProcessing:
		return 1, true
	case StatusSubmitted:
		return 2, true
	case StatusSent:
		return 3, true
	default:
		return 0, false
	}
}

func MapProviderStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case StatusQueued, StatusProcessing, StatusSubmitted, StatusSent, StatusDelivered, StatusUndelivered, StatusRejected, StatusFailed, StatusExpired, StatusUnknown, StatusCanceled:
		return status
	default:
		return StatusUnknown
	}
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	tenantContext, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return tenantContext, nil
}
