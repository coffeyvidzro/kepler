package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrNotFound = errors.New("email message not found")
var ErrNotCancelable = errors.New("email message is not a pending scheduled email")
var ErrSenderDomainNotFound = errors.New("sender domain not found")
var ErrActiveEmailTenantNotFound = errors.New("active email tenant not found")
var ErrSandboxTeamEmailNotVerified = platformemail.ErrSandboxTeamEmailNotVerified
var ErrSandboxRecipientRestricted = platformemail.ErrSandboxRecipientRestricted

type SenderDomainRoute struct {
	ID           uuid.UUID
	Provider     string
	Region       string
	Status       string
	HealthStatus string
	Disabled     bool
}

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.BeginTx(ctx, pgx.TxOptions{})
}

func (r *Repository) ResolveActiveCustomerRouteTx(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, provider, region, stream string) (platformemail.DeliveryRoute, error) {
	if tx == nil {
		return platformemail.DeliveryRoute{}, errors.New("email route transaction is required")
	}
	var tenantName string
	err := tx.QueryRow(ctx, `
		SELECT external_name
		FROM email_tenants
		WHERE team_id = $1
		  AND provider = $2
		  AND region = $3
		  AND status = 'active'
		FOR SHARE
	`, teamID, strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(region))).Scan(&tenantName)
	if errors.Is(err, pgx.ErrNoRows) {
		return platformemail.DeliveryRoute{}, ErrActiveEmailTenantNotFound
	}
	if err != nil {
		return platformemail.DeliveryRoute{}, fmt.Errorf("resolve active customer email tenant: %w", err)
	}
	route, err := platformemail.CustomerDeliveryRoute(stream, tenantName)
	if err != nil {
		return platformemail.DeliveryRoute{}, fmt.Errorf("build customer email route: %w", err)
	}
	return route, nil
}

func (r *Repository) AuthorizeSandboxRecipients(
	ctx context.Context,
	teamID uuid.UUID,
	to, cc, bcc []EmailAddress,
) error {
	if len(to) != 1 || len(cc) != 0 || len(bcc) != 0 {
		return platformemail.ErrSandboxRecipientRestricted
	}

	var ownerEmail string
	err := r.db.QueryRow(ctx, `
		SELECT owner.email
		FROM teams AS team
		JOIN team_members AS member
		  ON member.team_id = team.id
		 AND member.user_id = team.created_by
		 AND member.role = 'owner'
		 AND member.status = 'active'
		JOIN users AS owner
		  ON owner.id = team.created_by
		 AND owner.email_verified = true
		WHERE team.id = $1
		  AND team.status = 'active'
	`, teamID).Scan(&ownerEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return platformemail.ErrSandboxTeamEmailNotVerified
	}
	if err != nil {
		return fmt.Errorf("resolve verified sandbox owner email: %w", err)
	}
	return platformemail.ValidateSandboxRecipient(ownerEmail, to[0].Email, len(to), len(cc), len(bcc))
}

func (r *Repository) CreateTx(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, req validatedSend) (Message, error) {
	recipients, err := json.Marshal(map[string][]EmailAddress{"to": req.To, "cc": req.CC, "bcc": req.BCC, "reply_to": req.ReplyTo})
	if err != nil {
		return Message{}, fmt.Errorf("encode email recipients: %w", err)
	}
	headers, err := json.Marshal(platformemail.PersistDeliveryRoute(req.Headers, req.DeliveryRoute))
	if err != nil {
		return Message{}, fmt.Errorf("encode email headers: %w", err)
	}
	attachments, err := json.Marshal(req.Attachments)
	if err != nil {
		return Message{}, fmt.Errorf("encode email attachments: %w", err)
	}
	tags, err := json.Marshal(req.Tags)
	if err != nil {
		return Message{}, fmt.Errorf("encode email tags: %w", err)
	}
	row, err := r.queries.WithTx(tx).CreateEmailMessage(ctx, dbsqlc.CreateEmailMessageParams{
		TeamID:           teamID,
		SenderDomainID:   req.SenderDomainID,
		DeliveryProvider: req.Provider,
		ProviderRegion:   req.ProviderRegion,
		MessageType:      req.MessageType,
		FromEmail:        req.FromEmail,
		FromName:         req.FromName,
		ReplyToEmail:     req.ReplyToEmail,
		ToEmail:          req.ToEmail,
		ToName:           req.ToName,
		Subject:          req.Subject,
		HtmlBody:         req.HTMLBody,
		TextBody:         req.TextBody,
		Metadata:         req.Metadata,
		Recipients:       recipients,
		Headers:          headers,
		Attachments:      attachments,
		Tags:             tags,
		ScheduledAt:      pgconv.NullableTimestamptz(req.ScheduledAt),
	})
	if err != nil {
		return Message{}, fmt.Errorf("create email message: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) ResolveSenderDomain(ctx context.Context, teamID uuid.UUID, domainName string) (SenderDomainRoute, error) {
	row, err := r.queries.GetSenderDomainByDomain(ctx, dbsqlc.GetSenderDomainByDomainParams{
		TeamID: teamID,
		Domain: domainName,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SenderDomainRoute{}, ErrSenderDomainNotFound
	}
	if err != nil {
		return SenderDomainRoute{}, fmt.Errorf("resolve sender domain: %w", err)
	}
	status := row.Status
	if row.HealthStatus == "degraded" {
		status = "degraded"
	}
	return SenderDomainRoute{
		ID: row.ID, Provider: row.Provider, Region: row.ProviderRegion,
		Status: status, HealthStatus: row.HealthStatus, Disabled: row.DisabledAt.Valid,
	}, nil
}

func (r *Repository) CancelTx(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID) error {
	var status string
	var scheduledAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT status, scheduled_at
		FROM email_messages
		WHERE id = $1 AND team_id = $2
		FOR UPDATE
	`, id, teamID).Scan(&status, &scheduledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock email message for cancellation: %w", err)
	}
	if status != StatusQueued || scheduledAt == nil || !scheduledAt.After(time.Now().UTC()) {
		return ErrNotCancelable
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET status = $3, updated_at = now()
		WHERE id = $1 AND team_id = $2
	`, id, teamID, StatusCanceled); err != nil {
		return fmt.Errorf("cancel email message: %w", err)
	}
	return nil
}

func (r *Repository) RescheduleTx(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID, scheduledAt time.Time) error {
	var status string
	var currentSchedule *time.Time
	err := tx.QueryRow(ctx, `
		SELECT status, scheduled_at
		FROM email_messages
		WHERE id = $1 AND team_id = $2
		FOR UPDATE
	`, id, teamID).Scan(&status, &currentSchedule)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock email message for rescheduling: %w", err)
	}
	if status != StatusQueued || currentSchedule == nil || !currentSchedule.After(time.Now().UTC()) {
		return ErrNotCancelable
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_messages
		SET scheduled_at = $3, updated_at = now()
		WHERE id = $1 AND team_id = $2
	`, id, teamID, scheduledAt); err != nil {
		return fmt.Errorf("reschedule email message: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Message, error) {
	row, err := r.queries.GetEmailMessage(ctx, dbsqlc.GetEmailMessageParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("get email message: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]MessageSummary, error) {
	rows, err := r.queries.ListEmailMessages(ctx, dbsqlc.ListEmailMessagesParams{
		TeamID: teamID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list email messages: %w", err)
	}
	messages := make([]MessageSummary, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, MessageSummary{
			ID: row.ID.String(), ToEmail: row.ToEmail, ToName: row.ToName, Subject: row.Subject,
			Status: row.Status, Provider: row.Provider, QueuedAt: row.QueuedAt.Time,
			SubmittedAt: pgconv.TimestamptzToTimePtr(row.SubmittedAt), DeliveredAt: pgconv.TimestamptzToTimePtr(row.DeliveredAt),
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return messages, nil
}

func messageFromSQLC(row dbsqlc.EmailMessage) Message {
	message := Message{
		ID:                row.ID.String(),
		TeamID:            row.TeamID.String(),
		MessageType:       row.MessageType,
		FromEmail:         row.FromEmail,
		FromName:          row.FromName,
		ReplyToEmail:      row.ReplyToEmail,
		ToEmail:           row.ToEmail,
		ToName:            row.ToName,
		Subject:           row.Subject,
		HTMLBody:          row.HtmlBody,
		TextBody:          row.TextBody,
		Status:            row.Status,
		Provider:          row.Provider,
		ProviderMessageID: row.ProviderMessageID,
		ErrorCode:         row.ErrorCode,
		ErrorMessage:      row.ErrorMessage,
		Metadata:          json.RawMessage(row.Metadata),
		ScheduledAt:       pgconv.TimestamptzToTimePtr(row.ScheduledAt),
		QueuedAt:          row.QueuedAt.Time,
		ProcessingAt:      pgconv.TimestamptzToTimePtr(row.ProcessingAt),
		SubmittedAt:       pgconv.TimestamptzToTimePtr(row.SubmittedAt),
		DeliveredAt:       pgconv.TimestamptzToTimePtr(row.DeliveredAt),
		FailedAt:          pgconv.TimestamptzToTimePtr(row.FailedAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
	var recipients struct {
		To      []EmailAddress `json:"to"`
		CC      []EmailAddress `json:"cc"`
		BCC     []EmailAddress `json:"bcc"`
		ReplyTo []EmailAddress `json:"reply_to"`
	}
	_ = json.Unmarshal(row.Recipients, &recipients)
	message.To, message.CC, message.BCC, message.ReplyTo = recipients.To, recipients.CC, recipients.BCC, recipients.ReplyTo
	var persistedHeaders map[string]string
	_ = json.Unmarshal(row.Headers, &persistedHeaders)
	_, message.Headers = platformemail.ExtractDeliveryRoute(persistedHeaders)
	_ = json.Unmarshal(row.Attachments, &message.Attachments)
	_ = json.Unmarshal(row.Tags, &message.Tags)
	return message
}
