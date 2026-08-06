package messagingfeedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
	platformfeedback "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/feedback"
)

const processedConsumerName = "messaging-feedback-v1"

var processedEventNamespace = uuid.MustParse("35b15a58-9f5a-5da7-b061-83ba967bd4c9")

// Repository applies normalized feedback inside a caller-owned transaction.
type Repository struct {
	tx pgx.Tx
}

func NewRepository(tx pgx.Tx) *Repository {
	return &Repository{tx: tx}
}

func (repository *Repository) FindAttempt(ctx context.Context, lookup platformfeedback.Lookup) (delivery.Attempt, error) {
	if repository == nil || repository.tx == nil {
		return delivery.Attempt{}, errors.New("messaging feedback repository is not configured")
	}
	provider := strings.ToLower(strings.TrimSpace(lookup.Provider))
	providerMessageID := strings.TrimSpace(lookup.ProviderMessageID)
	if lookup.AttemptID != uuid.Nil {
		return scanAttempt(repository.tx.QueryRow(ctx, `
			SELECT id, team_id, channel, email_message_id, sms_message_id,
				attempt_number, status, COALESCE(provider, ''), provider_account,
				COALESCE(provider_message_id, ''), COALESCE(provider_status, ''),
				sender_asset_id, sender_provider_binding_id,
				COALESCE(error_code, ''), COALESCE(error_message, ''),
				claimed_at, request_started_at, request_completed_at, submitted_at,
				terminal_at, next_reconcile_at, last_reconciled_at,
				reconcile_attempts, metadata, created_at, updated_at
			FROM message_delivery_attempts
			WHERE id = $1
			  AND channel = $2
			  AND lower(COALESCE(provider, '')) = $3
			  AND provider_message_id = $4
			FOR UPDATE
		`, lookup.AttemptID, string(lookup.Channel), provider, providerMessageID))
	}
	return scanAttempt(repository.tx.QueryRow(ctx, `
		SELECT id, team_id, channel, email_message_id, sms_message_id,
			attempt_number, status, COALESCE(provider, ''), provider_account,
			COALESCE(provider_message_id, ''), COALESCE(provider_status, ''),
			sender_asset_id, sender_provider_binding_id,
			COALESCE(error_code, ''), COALESCE(error_message, ''),
			claimed_at, request_started_at, request_completed_at, submitted_at,
			terminal_at, next_reconcile_at, last_reconciled_at,
			reconcile_attempts, metadata, created_at, updated_at
		FROM message_delivery_attempts
		WHERE channel = $1
		  AND lower(COALESCE(provider, '')) = $2
		  AND provider_message_id = $3
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, string(lookup.Channel), provider, providerMessageID))
}

func scanAttempt(row pgx.Row) (delivery.Attempt, error) {
	var attempt delivery.Attempt
	var channel string
	var status string
	var metadata []byte
	var emailMessageID pgtype.UUID
	var smsMessageID pgtype.UUID
	var senderAssetID pgtype.UUID
	var senderBindingID pgtype.UUID
	var requestStartedAt pgtype.Timestamptz
	var requestCompletedAt pgtype.Timestamptz
	var submittedAt pgtype.Timestamptz
	var terminalAt pgtype.Timestamptz
	var nextReconcileAt pgtype.Timestamptz
	var lastReconciledAt pgtype.Timestamptz
	if err := row.Scan(
		&attempt.ID,
		&attempt.TeamID,
		&channel,
		&emailMessageID,
		&smsMessageID,
		&attempt.AttemptNumber,
		&status,
		&attempt.Provider,
		&attempt.ProviderAccount,
		&attempt.ProviderMessageID,
		&attempt.ProviderStatus,
		&senderAssetID,
		&senderBindingID,
		&attempt.ErrorCode,
		&attempt.ErrorMessage,
		&attempt.ClaimedAt,
		&requestStartedAt,
		&requestCompletedAt,
		&submittedAt,
		&terminalAt,
		&nextReconcileAt,
		&lastReconciledAt,
		&attempt.ReconcileAttempts,
		&metadata,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return delivery.Attempt{}, platformfeedback.ErrAttemptNotFound
		}
		return delivery.Attempt{}, fmt.Errorf("scan delivery attempt for feedback: %w", err)
	}
	attempt.Channel = messaging.Channel(channel)
	attempt.Status = delivery.AttemptStatus(status)
	attempt.EmailMessageID = uuidPointer(emailMessageID)
	attempt.SMSMessageID = uuidPointer(smsMessageID)
	attempt.SenderAssetID = uuidPointer(senderAssetID)
	attempt.SenderProviderBindingID = uuidPointer(senderBindingID)
	attempt.RequestStartedAt = timePointer(requestStartedAt)
	attempt.RequestCompletedAt = timePointer(requestCompletedAt)
	attempt.SubmittedAt = timePointer(submittedAt)
	attempt.TerminalAt = timePointer(terminalAt)
	attempt.NextReconcileAt = timePointer(nextReconcileAt)
	attempt.LastReconciledAt = timePointer(lastReconciledAt)
	attempt.Metadata = append(json.RawMessage(nil), metadata...)
	return attempt, nil
}

func uuidPointer(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func (repository *Repository) ApplyEvent(
	ctx context.Context,
	event platformfeedback.Event,
	update platformfeedback.AttemptUpdate,
) (platformfeedback.ApplyResult, error) {
	if repository == nil || repository.tx == nil {
		return platformfeedback.ApplyResult{}, errors.New("messaging feedback repository is not configured")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return platformfeedback.ApplyResult{}, fmt.Errorf("encode normalized feedback event: %w", err)
	}
	tag, err := repository.tx.Exec(ctx, `
		INSERT INTO processed_events (consumer_name, event_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (consumer_name, event_id) DO NOTHING
	`, processedConsumerName, processedEventID(event), payload)
	if err != nil {
		return platformfeedback.ApplyResult{}, fmt.Errorf("deduplicate normalized feedback event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return platformfeedback.ApplyResult{Duplicate: true}, nil
	}

	if update.Status == nil {
		tag, err = repository.tx.Exec(ctx, `
			UPDATE message_delivery_attempts
			SET last_reconciled_at = $3,
				reconcile_attempts = reconcile_attempts + 1,
				updated_at = now()
			WHERE id = $1 AND status = $2
		`, update.AttemptID, string(update.ExpectedStatus), update.ReconciledAt)
	} else {
		tag, err = repository.tx.Exec(ctx, `
			UPDATE message_delivery_attempts
			SET status = $3,
				provider_status = CASE WHEN trim($4) = '' THEN provider_status ELSE $4 END,
				error_code = NULLIF(trim($5), ''),
				error_message = NULLIF(trim($6), ''),
				request_completed_at = COALESCE(request_completed_at, $8),
				submitted_at = CASE
					WHEN $3 IN ('submitted', 'accepted', 'sent', 'delivered')
					THEN COALESCE(submitted_at, $7)
					ELSE submitted_at
				END,
				terminal_at = COALESCE(terminal_at, $9),
				last_reconciled_at = $8,
				reconcile_attempts = reconcile_attempts + 1,
				next_reconcile_at = CASE
					WHEN $3 IN ('delivered', 'permanent_failure', 'rejected', 'expired', 'canceled') THEN NULL
					ELSE next_reconcile_at
				END,
				updated_at = now()
			WHERE id = $1 AND status = $2
		`,
			update.AttemptID,
			string(update.ExpectedStatus),
			string(*update.Status),
			update.ProviderStatus,
			update.ErrorCode,
			update.ErrorMessage,
			update.OccurredAt,
			update.ReconciledAt,
			update.TerminalAt,
		)
	}
	if err != nil {
		return platformfeedback.ApplyResult{}, fmt.Errorf("update delivery attempt from feedback: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return platformfeedback.ApplyResult{}, platformfeedback.ErrConcurrentUpdate
	}
	transitioned := update.Status != nil && *update.Status != update.ExpectedStatus
	return platformfeedback.ApplyResult{Applied: true, Transitioned: transitioned}, nil
}

func processedEventID(event platformfeedback.Event) uuid.UUID {
	return uuid.NewSHA1(processedEventNamespace, []byte(event.DedupeKey()))
}
