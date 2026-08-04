package emaildelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

var ErrMessageNotDeliverable = errors.New("email message is not deliverable")
var ErrSenderDomainUnavailable = errors.New("sender domain is no longer available for delivery")

type DeliveryMessage struct {
	ID          uuid.UUID
	TeamID      uuid.UUID
	AttemptID   uuid.UUID
	Provider    string
	Region      string
	FromEmail   string
	FromName    string
	ReplyTo     []platformemail.Address
	To          []platformemail.Address
	CC          []platformemail.Address
	BCC         []platformemail.Address
	Subject     string
	HTML        string
	Text        string
	Headers     map[string]string
	Attachments []platformemail.Attachment
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Claim(ctx context.Context, messageID, teamID uuid.UUID) (DeliveryMessage, error) {
	if r == nil || r.db == nil {
		return DeliveryMessage{}, errors.New("email delivery repository is not configured")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return DeliveryMessage{}, fmt.Errorf("begin email delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var message DeliveryMessage
	var fromName *string
	var htmlBody, textBody *string
	var recipientsJSON, headersJSON, attachmentsJSON []byte
	var authorized bool
	err = tx.QueryRow(ctx, `
		SELECT message.id, message.team_id, message.delivery_provider, message.provider_region,
			message.from_email, message.from_name, message.subject, message.html_body, message.text_body,
			message.recipients, message.headers, message.attachments,
			message.sender_domain_id IS NULL OR EXISTS (
				SELECT 1
				FROM sender_domains AS domain
				WHERE domain.id = message.sender_domain_id
				  AND domain.team_id = message.team_id
				  AND domain.status = 'verified'
				  AND domain.disabled_at IS NULL
				  AND domain.health_status <> 'degraded'
			) AS authorized
		FROM email_messages AS message
		WHERE message.id = $1
		  AND message.team_id = $2
		  AND message.status = 'queued'
		FOR UPDATE OF message
	`, messageID, teamID).Scan(
		&message.ID, &message.TeamID, &message.Provider, &message.Region, &message.FromEmail, &fromName,
		&message.Subject, &htmlBody, &textBody, &recipientsJSON, &headersJSON, &attachmentsJSON, &authorized,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryMessage{}, ErrMessageNotDeliverable
	}
	if err != nil {
		return DeliveryMessage{}, fmt.Errorf("lock email message for delivery: %w", err)
	}

	if fromName != nil {
		message.FromName = *fromName
	}
	if htmlBody != nil {
		message.HTML = *htmlBody
	}
	if textBody != nil {
		message.Text = *textBody
	}
	var recipients struct {
		To      []platformemail.Address `json:"to"`
		CC      []platformemail.Address `json:"cc"`
		BCC     []platformemail.Address `json:"bcc"`
		ReplyTo []platformemail.Address `json:"reply_to"`
	}
	if err := json.Unmarshal(recipientsJSON, &recipients); err != nil {
		return DeliveryMessage{}, fmt.Errorf("decode email recipients: %w", err)
	}
	message.To, message.CC, message.BCC, message.ReplyTo = recipients.To, recipients.CC, recipients.BCC, recipients.ReplyTo
	if err := json.Unmarshal(headersJSON, &message.Headers); err != nil {
		return DeliveryMessage{}, fmt.Errorf("decode email headers: %w", err)
	}
	if err := json.Unmarshal(attachmentsJSON, &message.Attachments); err != nil {
		return DeliveryMessage{}, fmt.Errorf("decode email attachments: %w", err)
	}

	if !authorized {
		if _, err := tx.Exec(ctx, `
			UPDATE email_messages
			SET status = 'failed', error_code = 'sender_domain_unavailable',
				error_message = 'Sender domain is no longer verified, enabled, and healthy',
				failed_at = now(), updated_at = now()
			WHERE id = $1 AND team_id = $2 AND status = 'queued'
		`, messageID, teamID); err != nil {
			return DeliveryMessage{}, fmt.Errorf("fail unavailable sender domain: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return DeliveryMessage{}, fmt.Errorf("commit unavailable sender domain: %w", err)
		}
		return DeliveryMessage{}, ErrSenderDomainUnavailable
	}

	message.AttemptID = uuid.New()
	var attemptNumber int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_number), 0) + 1
		FROM email_delivery_attempts
		WHERE email_message_id = $1
	`, messageID).Scan(&attemptNumber); err != nil {
		return DeliveryMessage{}, fmt.Errorf("calculate email delivery attempt number: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_delivery_attempts (
			id, email_message_id, team_id, attempt_number, status, provider
		)
		VALUES ($1, $2, $3, $4, 'claimed', $5)
	`, message.AttemptID, messageID, teamID, attemptNumber, message.Provider); err != nil {
		return DeliveryMessage{}, fmt.Errorf("create email delivery attempt: %w", err)
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET status = 'processing', current_delivery_attempt_id = $3,
			processing_at = now(), error_code = NULL, error_message = NULL, updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'queued'
	`, messageID, teamID, message.AttemptID)
	if err != nil {
		return DeliveryMessage{}, fmt.Errorf("claim email message: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return DeliveryMessage{}, ErrMessageNotDeliverable
	}
	if err := tx.Commit(ctx); err != nil {
		return DeliveryMessage{}, fmt.Errorf("commit email delivery claim: %w", err)
	}
	return message, nil
}

func (r *Repository) MarkRequestStarted(ctx context.Context, messageID, teamID, attemptID uuid.UUID) error {
	commandTag, err := r.db.Exec(ctx, `
		UPDATE email_delivery_attempts AS attempt
		SET status = 'request_started', request_started_at = now(), updated_at = now()
		FROM email_messages AS message
		WHERE attempt.id = $3
		  AND attempt.email_message_id = $1
		  AND attempt.team_id = $2
		  AND attempt.status = 'claimed'
		  AND message.id = attempt.email_message_id
		  AND message.team_id = attempt.team_id
		  AND message.status = 'processing'
		  AND message.current_delivery_attempt_id = attempt.id
	`, messageID, teamID, attemptID)
	if err != nil {
		return fmt.Errorf("mark provider request started: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrMessageNotDeliverable
	}
	return nil
}

func (r *Repository) MarkSubmitted(ctx context.Context, messageID, teamID, attemptID uuid.UUID, result platformemail.Result) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin submitted email transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	commandTag, err := tx.Exec(ctx, `
		UPDATE email_delivery_attempts
		SET status = 'submitted', provider = $4, provider_message_id = $5,
			completed_at = now(), error_code = NULL, error_message = NULL, updated_at = now()
		WHERE id = $3 AND email_message_id = $1 AND team_id = $2
		  AND status = 'request_started'
	`, messageID, teamID, attemptID, result.Provider, result.MessageID)
	if err != nil {
		return fmt.Errorf("mark email attempt submitted: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrMessageNotDeliverable
	}
	commandTag, err = tx.Exec(ctx, `
		UPDATE email_messages
		SET status = 'submitted', provider = $4, provider_message_id = $5,
			submitted_at = now(), error_code = NULL, error_message = NULL, updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'processing'
		  AND current_delivery_attempt_id = $3
	`, messageID, teamID, attemptID, result.Provider, result.MessageID)
	if err != nil {
		return fmt.Errorf("mark email submitted: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrMessageNotDeliverable
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit submitted email transaction: %w", err)
	}
	return nil
}

func (r *Repository) MarkRetryable(ctx context.Context, messageID, teamID, attemptID uuid.UUID, cause error) error {
	return r.completeAttempt(ctx, messageID, teamID, attemptID, "retryable_failure", "queued", "provider_retryable", cause)
}

func (r *Repository) MarkSubmissionUnknown(ctx context.Context, messageID, teamID, attemptID uuid.UUID, code string, cause error) error {
	if code == "" {
		code = "submission_unknown"
	}
	return r.completeAttempt(ctx, messageID, teamID, attemptID, "submission_unknown", "submission_unknown", code, cause)
}

func (r *Repository) MarkFailed(ctx context.Context, messageID, teamID, attemptID uuid.UUID, code string, cause error) error {
	return r.completeAttempt(ctx, messageID, teamID, attemptID, "failed", "failed", code, cause)
}

func (r *Repository) completeAttempt(
	ctx context.Context,
	messageID, teamID, attemptID uuid.UUID,
	attemptStatus, messageStatus, code string,
	cause error,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email attempt completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	commandTag, err := tx.Exec(ctx, `
		UPDATE email_delivery_attempts
		SET status = $4, error_code = $5, error_message = $6,
			completed_at = now(), updated_at = now()
		WHERE id = $3 AND email_message_id = $1 AND team_id = $2
		  AND status IN ('claimed', 'request_started')
	`, messageID, teamID, attemptID, attemptStatus, code, truncateError(cause))
	if err != nil {
		return fmt.Errorf("complete email delivery attempt: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrMessageNotDeliverable
	}

	failedAtExpression := "NULL"
	if messageStatus == "failed" {
		failedAtExpression = "now()"
	}
	query := fmt.Sprintf(`
		UPDATE email_messages
		SET status = $4, error_code = $5, error_message = $6,
			processing_at = CASE WHEN $4 = 'queued' THEN NULL ELSE processing_at END,
			current_delivery_attempt_id = CASE WHEN $4 = 'queued' THEN NULL ELSE current_delivery_attempt_id END,
			failed_at = %s, updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'processing'
		  AND current_delivery_attempt_id = $3
	`, failedAtExpression)
	commandTag, err = tx.Exec(ctx, query, messageID, teamID, attemptID, messageStatus, code, truncateError(cause))
	if err != nil {
		return fmt.Errorf("complete email message delivery state: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrMessageNotDeliverable
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email attempt completion: %w", err)
	}
	return nil
}

func (r *Repository) MarkExhausted(ctx context.Context, messageID, teamID uuid.UUID, cause error) error {
	_, err := r.db.Exec(ctx, `
		UPDATE email_messages
		SET status = 'failed', error_code = 'retry_exhausted', error_message = $3,
			failed_at = now(), updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'queued'
	`, messageID, teamID, truncateError(cause))
	if err != nil {
		return fmt.Errorf("mark email retries exhausted: %w", err)
	}
	return nil
}

func truncateError(err error) string {
	if err == nil {
		return "unknown email delivery failure"
	}
	value := err.Error()
	const maxLength = 2000
	if len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}

func (r *Repository) ResetStaleProcessing(ctx context.Context, olderThan time.Time) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin stale email reset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	unknownTag, err := tx.Exec(ctx, `
		WITH stale AS (
			SELECT message.id, message.team_id, message.current_delivery_attempt_id
			FROM email_messages AS message
			JOIN email_delivery_attempts AS attempt
			  ON attempt.id = message.current_delivery_attempt_id
			WHERE message.status = 'processing'
			  AND message.processing_at < $1
			  AND attempt.status = 'request_started'
			FOR UPDATE OF message, attempt
		), updated_attempts AS (
			UPDATE email_delivery_attempts AS attempt
			SET status = 'submission_unknown', error_code = 'worker_interrupted',
				error_message = 'Worker stopped after the provider request started',
				completed_at = now(), updated_at = now()
			FROM stale
			WHERE attempt.id = stale.current_delivery_attempt_id
			RETURNING stale.id, stale.team_id, stale.current_delivery_attempt_id
		)
		UPDATE email_messages AS message
		SET status = 'submission_unknown', error_code = 'worker_interrupted',
			error_message = 'Worker stopped after the provider request started', updated_at = now()
		FROM updated_attempts
		WHERE message.id = updated_attempts.id
		  AND message.team_id = updated_attempts.team_id
		  AND message.current_delivery_attempt_id = updated_attempts.current_delivery_attempt_id
	`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("mark stale provider submissions unknown: %w", err)
	}

	retryTag, err := tx.Exec(ctx, `
		WITH stale AS (
			SELECT message.id, message.team_id, message.current_delivery_attempt_id
			FROM email_messages AS message
			JOIN email_delivery_attempts AS attempt
			  ON attempt.id = message.current_delivery_attempt_id
			WHERE message.status = 'processing'
			  AND message.processing_at < $1
			  AND attempt.status = 'claimed'
			FOR UPDATE OF message, attempt
		), updated_attempts AS (
			UPDATE email_delivery_attempts AS attempt
			SET status = 'retryable_failure', error_code = 'worker_interrupted_before_request',
				error_message = 'Worker stopped before the provider request started',
				completed_at = now(), updated_at = now()
			FROM stale
			WHERE attempt.id = stale.current_delivery_attempt_id
			RETURNING stale.id, stale.team_id, stale.current_delivery_attempt_id
		)
		UPDATE email_messages AS message
		SET status = 'queued', current_delivery_attempt_id = NULL, processing_at = NULL,
			error_code = 'worker_interrupted_before_request',
			error_message = 'Worker stopped before the provider request started', updated_at = now()
		FROM updated_attempts
		WHERE message.id = updated_attempts.id
		  AND message.team_id = updated_attempts.team_id
		  AND message.current_delivery_attempt_id = updated_attempts.current_delivery_attempt_id
	`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("reset stale unstarted email attempts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit stale email reset: %w", err)
	}
	return unknownTag.RowsAffected() + retryTag.RowsAffected(), nil
}
