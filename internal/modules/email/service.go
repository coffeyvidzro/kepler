package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	maxBatchSize         = 50
	maxBatchPayloadBytes = 10 << 20
)

type DeliveryQueue interface {
	EnqueueEmailDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error
}
type scheduledDeliveryQueue interface {
	EnqueueEmailDeliveryAtTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) error
	CancelEmailDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error
	RescheduleEmailDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) error
}

type QueuedMessage struct {
	Message Message
	Charge  platformbilling.CommittedCharge
}

func (s *Service) Update(ctx context.Context, value string, req UpdateRequest) (MutationResponse, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailSend)
	if err != nil {
		return MutationResponse{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return MutationResponse{}, apperrors.NewBadRequest("Email message id must be a valid UUID")
	}
	scheduledAt, err := normalizeUpdateSchedule(req.ScheduledAt)
	if err != nil {
		return MutationResponse{}, err
	}
	queue, ok := s.delivery.(scheduledDeliveryQueue)
	if !ok {
		return MutationResponse{}, apperrors.NewInternal("Email delivery queue does not support rescheduling", nil)
	}
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to begin email update transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.repository.RescheduleTx(ctx, tx, id, tc.Scope.TeamID, scheduledAt); err != nil {
		if errors.Is(err, ErrNotFound) {
			return MutationResponse{}, apperrors.NewNotFound("Email message not found")
		}
		if errors.Is(err, ErrNotCancelable) {
			return MutationResponse{}, apperrors.NewConflict("Only pending scheduled emails can be updated")
		}
		return MutationResponse{}, apperrors.NewInternal("Unable to update email message", err)
	}
	if err := queue.RescheduleEmailDeliveryTx(ctx, tx, id, tc.Scope.TeamID, scheduledAt); err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to reschedule email delivery", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to commit email update", err)
	}
	return MutationResponse{Object: "email", ID: id.String()}, nil
}

func (s *Service) Cancel(ctx context.Context, value string) (MutationResponse, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailSend)
	if err != nil {
		return MutationResponse{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return MutationResponse{}, apperrors.NewBadRequest("Email message id must be a valid UUID")
	}
	queue, ok := s.delivery.(scheduledDeliveryQueue)
	if !ok {
		return MutationResponse{}, apperrors.NewInternal("Email delivery queue does not support cancellation", nil)
	}
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to begin email cancellation transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.repository.CancelTx(ctx, tx, id, tc.Scope.TeamID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return MutationResponse{}, apperrors.NewNotFound("Email message not found")
		}
		if errors.Is(err, ErrNotCancelable) {
			return MutationResponse{}, apperrors.NewConflict("Only pending scheduled emails can be canceled")
		}
		return MutationResponse{}, apperrors.NewInternal("Unable to cancel email message", err)
	}
	if err := queue.CancelEmailDeliveryTx(ctx, tx, id, tc.Scope.TeamID); err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to cancel email delivery", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MutationResponse{}, apperrors.NewInternal("Unable to commit email cancellation", err)
	}
	return MutationResponse{Object: "email", ID: id.String()}, nil
}

type Service struct {
	repository    *Repository
	delivery      DeliveryQueue
	config        ServiceConfig
	senderDomains senderDomainResolver
	routes        customerRouteResolver
	billing       platformbilling.EmailBilling
}

type senderDomainResolver interface {
	ResolveSenderDomain(context.Context, uuid.UUID, string) (SenderDomainRoute, error)
}

type customerRouteResolver interface {
	ResolveActiveCustomerRouteTx(context.Context, pgx.Tx, uuid.UUID, string, string, string) (platformemail.DeliveryRoute, error)
}

type ServiceConfig struct {
	DefaultFromEmail string
	DefaultFromName  string
	DefaultProvider  string
	DefaultRegion    string
}

func NewService(
	repository *Repository,
	delivery DeliveryQueue,
	config ServiceConfig,
	billing platformbilling.EmailBilling,
	dependencies ...any,
) *Service {
	service := &Service{
		repository: repository, delivery: delivery, config: config,
		senderDomains: repository, routes: repository, billing: billing,
	}
	for _, dependency := range dependencies {
		if resolver, ok := dependency.(senderDomainResolver); ok {
			service.senderDomains = resolver
		}
		if resolver, ok := dependency.(customerRouteResolver); ok {
			service.routes = resolver
		}
	}
	return service
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	tc, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return tc, nil
}

func (s *Service) Send(ctx context.Context, req SendRequest) (Message, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailSend)
	if err != nil {
		return Message{}, err
	}
	if s == nil || s.repository == nil {
		return Message{}, apperrors.NewInternal("Customer email routing is not configured", nil)
	}
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to begin email transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queued, err := s.EnqueueTx(ctx, tx, tc.Scope.TeamID, req)
	if err != nil {
		return Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Message{}, apperrors.NewInternal("Unable to commit email transaction", err)
	}
	s.ObserveCommitted(ctx, queued)
	return queued.Message, nil
}

func (s *Service) EnqueueTx(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, req SendRequest) (QueuedMessage, error) {
	validated, err := s.prepareSend(ctx, teamID, req)
	if err != nil {
		return QueuedMessage{}, err
	}
	return s.enqueueValidatedTx(ctx, tx, teamID, validated)
}

func (s *Service) ObserveCommitted(ctx context.Context, queued QueuedMessage) {
	if s == nil || s.billing == nil {
		return
	}
	s.billing.ObserveCommittedCharge(ctx, queued.Charge)
}

func (s *Service) Get(ctx context.Context, value string) (Message, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailRead)
	if err != nil {
		return Message{}, err
	}
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return Message{}, apperrors.NewBadRequest("Email message id must be a valid UUID")
	}
	m, err := s.repository.Get(ctx, id, tc.Scope.TeamID)
	if errors.Is(err, ErrNotFound) {
		return Message{}, apperrors.NewNotFound("Email message not found")
	}
	if err != nil {
		return Message{}, apperrors.NewInternal("Unable to get email message", err)
	}
	return m, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]MessageSummary, error) {
	tc, err := requireTenant(ctx, tenant.PermissionEmailRead)
	if err != nil {
		return nil, err
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	m, err := s.repository.List(ctx, tc.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list email messages", err)
	}
	return m, nil
}

func (s *Service) BatchSend(ctx context.Context, req BatchSendRequest) ([]Message, error) {
	if len(req.Messages) == 0 || len(req.Messages) > maxBatchSize {
		return nil, apperrors.NewBadRequest(fmt.Sprintf("batch must contain between 1 and %d emails", maxBatchSize))
	}
	tc, err := requireTenant(ctx, tenant.PermissionEmailSend)
	if err != nil {
		return nil, err
	}
	if err := s.ensureEnqueueConfigured(); err != nil {
		return nil, err
	}

	validated := make([]validatedSend, len(req.Messages))
	totalPayloadBytes := 0
	for index, item := range req.Messages {
		if len(item.Attachments) > 0 {
			return nil, apperrors.NewBadRequest("attachments are not supported in batch emails")
		}
		validated[index], err = s.prepareSend(ctx, tc.Scope.TeamID, item)
		if err != nil {
			return nil, err
		}
		totalPayloadBytes += bodySize(validated[index].HTMLBody) + bodySize(validated[index].TextBody) + len(validated[index].Metadata)
		if totalPayloadBytes > maxBatchPayloadBytes {
			return nil, apperrors.NewPayloadTooLarge("Email batch payload is too large")
		}
	}

	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to begin email batch transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result := make([]Message, 0, len(validated))
	queuedMessages := make([]QueuedMessage, 0, len(validated))
	for index := range validated {
		queued, enqueueErr := s.enqueueValidatedTx(ctx, tx, tc.Scope.TeamID, validated[index])
		if enqueueErr != nil {
			return nil, enqueueErr
		}
		result = append(result, queued.Message)
		queuedMessages = append(queuedMessages, queued)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.NewInternal("Unable to commit email batch transaction", err)
	}
	for _, queued := range queuedMessages {
		s.ObserveCommitted(ctx, queued)
	}
	return result, nil
}

func (s *Service) prepareSend(ctx context.Context, teamID uuid.UUID, req SendRequest) (validatedSend, error) {
	validated, err := validateSend(req, s.config)
	if err != nil {
		return validatedSend{}, err
	}
	if err := s.authorizeSender(ctx, teamID, &validated); err != nil {
		return validatedSend{}, err
	}
	if err := s.ensureEnqueueConfigured(); err != nil {
		return validatedSend{}, err
	}
	return validated, nil
}

func (s *Service) ensureEnqueueConfigured() error {
	if s == nil || s.delivery == nil {
		return apperrors.NewInternal("Email delivery queue is not configured", nil)
	}
	if s.repository == nil || s.routes == nil {
		return apperrors.NewInternal("Customer email routing is not configured", nil)
	}
	if s.billing == nil {
		return apperrors.NewInternal("Email billing charge is not configured", nil)
	}
	return nil
}

func (s *Service) enqueueValidatedTx(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, validated validatedSend) (QueuedMessage, error) {
	if tx == nil {
		return QueuedMessage{}, apperrors.NewInternal("Email transaction is not configured", nil)
	}
	if validated.DeliveryRoute.SESTenantName == "" {
		var err error
		validated.DeliveryRoute, err = s.routes.ResolveActiveCustomerRouteTx(ctx, tx, teamID, validated.Provider, validated.ProviderRegion, validated.MessageType)
		if errors.Is(err, ErrActiveEmailTenantNotFound) {
			return QueuedMessage{}, apperrors.NewConflict("Customer email tenant is not active")
		}
		if err != nil {
			return QueuedMessage{}, apperrors.NewInternal("Unable to resolve customer email route", err)
		}
	}
	message, err := s.repository.CreateTx(ctx, tx, teamID, validated)
	if err != nil {
		return QueuedMessage{}, apperrors.NewInternal("Unable to create email message", err)
	}
	messageID := uuid.MustParse(message.ID)
	// Billing settles when the message and its durable delivery job commit, not
	// when the provider later accepts or delivers the email.
	charge, err := s.billing.ChargeEmail(ctx, tx, platformbilling.EmailChargeInput{
		TeamID: teamID, MessageID: messageID,
		RecipientCount: emailRecipientCount(validated),
	})
	if err != nil {
		return QueuedMessage{}, emailBillingError(err)
	}
	if err := enqueueDelivery(ctx, s.delivery, tx, messageID, teamID, validated.ScheduledAt); err != nil {
		return QueuedMessage{}, apperrors.NewInternal("Unable to enqueue email delivery", err)
	}
	return QueuedMessage{
		Message: message,
		Charge: platformbilling.CommittedCharge{
			Charge: charge, Channel: platformbilling.ChannelEmail,
			Settlement: platformbilling.SettlementAcceptedForDelivery,
			TeamID:     teamID, MessageID: messageID,
		},
	}, nil
}

func emailBillingError(err error) error {
	switch {
	case errors.Is(err, platformbilling.ErrInsufficientBalance):
		return apperrors.NewPaymentRequired("Insufficient wallet balance")
	case errors.Is(err, platformbilling.ErrTeamNotFound):
		return apperrors.NewNotFound("Billing team not found")
	case errors.Is(err, platformbilling.ErrTeamInactive):
		return apperrors.NewConflict("Team is not active for billing")
	case errors.Is(err, platformbilling.ErrUnsupportedMarket):
		return apperrors.NewConflict("Team market is not supported for billing")
	case errors.Is(err, platformbilling.ErrWalletNotFound):
		return apperrors.NewConflict("Team wallet is not initialized")
	case errors.Is(err, platformbilling.ErrRateNotFound):
		return apperrors.NewServiceUnavailable("Email pricing is unavailable", err)
	case errors.Is(err, platformbilling.ErrCurrencyMismatch):
		return apperrors.NewConflict("Wallet currency does not match the team market")
	case errors.Is(err, platformbilling.ErrAmountOverflow):
		return apperrors.NewInternal("Email charge amount exceeds the supported range", err)
	default:
		return apperrors.NewInternal("Unable to apply email billing charge", err)
	}
}

func (s *Service) authorizeSender(ctx context.Context, teamID uuid.UUID, message *validatedSend) error {
	provider := strings.TrimSpace(s.config.DefaultProvider)
	region := strings.TrimSpace(s.config.DefaultRegion)
	if provider == "" || region == "" {
		return apperrors.NewInternal("Email delivery route is not configured", nil)
	}
	separator := strings.LastIndexByte(message.FromEmail, '@')
	if separator < 0 || separator == len(message.FromEmail)-1 {
		return apperrors.NewBadRequest("Email sender is invalid")
	}
	if s.senderDomains == nil {
		return apperrors.NewInternal("Sender domain repository is not configured", nil)
	}
	route, err := s.senderDomains.ResolveSenderDomain(ctx, teamID, strings.ToLower(message.FromEmail[separator+1:]))
	if errors.Is(err, ErrSenderDomainNotFound) {
		return apperrors.NewForbidden("Email sender domain is not authorized for this team")
	}
	if err != nil {
		return apperrors.NewInternal("Unable to authorize email sender domain", err)
	}
	if route.Status != "verified" || route.Disabled {
		return apperrors.NewConflict("Email sender domain is not verified and enabled")
	}
	if !strings.EqualFold(strings.TrimSpace(route.Provider), provider) || strings.TrimSpace(route.Region) == "" {
		return apperrors.NewInternal("Sender domain delivery route is invalid", nil)
	}
	message.SenderDomainID = &route.ID
	message.Provider = route.Provider
	message.ProviderRegion = route.Region
	return nil
}

func enqueueDelivery(ctx context.Context, queue DeliveryQueue, tx pgx.Tx, messageID, teamID uuid.UUID, scheduledAt *time.Time) error {
	if scheduledAt != nil {
		if scheduled, ok := queue.(scheduledDeliveryQueue); ok {
			return scheduled.EnqueueEmailDeliveryAtTx(ctx, tx, messageID, teamID, *scheduledAt)
		}
		return errors.New("email delivery queue does not support scheduled delivery")
	}
	return queue.EnqueueEmailDeliveryTx(ctx, tx, messageID, teamID)
}

func emailRecipientCount(message validatedSend) int64 {
	return int64(len(message.To) + len(message.CC) + len(message.BCC))
}

func bodySize(body *string) int {
	if body == nil {
		return 0
	}
	return len(*body)
}
