package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	emailfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/outbound"
	smsfeedback "github.com/coffeyvidzro/dugble/server/internal/delivery/sms/feedback"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	webhooks "github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformdelivery "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
	platformfeedback "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/feedback"
	"github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

type checkingEmailSender struct {
	pool      *pgxpool.Pool
	attemptID uuid.UUID
	calls     int
}

func (sender *checkingEmailSender) Send(ctx context.Context, _ awsses.Message) (awsses.Result, error) {
	status := requireAttemptStatus(ctx, sender.pool, sender.attemptID)
	if status != string(platformdelivery.StatusRequestStarted) {
		return awsses.Result{}, fmt.Errorf("email provider called before request_started attempt; status=%s", status)
	}
	sender.calls++
	return awsses.Result{Provider: "ses", MessageID: "ses-message-e2e"}, nil
}

type checkingSMSProvider struct {
	pool      *pgxpool.Pool
	attemptID uuid.UUID
	calls     int
}

func (provider *checkingSMSProvider) ID() string { return "mnotify" }

func (provider *checkingSMSProvider) Send(ctx context.Context, _ sms.SendRequest) (*sms.SendResponse, error) {
	status := requireAttemptStatus(ctx, provider.pool, provider.attemptID)
	if status != string(platformdelivery.StatusRequestStarted) {
		return nil, fmt.Errorf("SMS provider called before request_started attempt; status=%s", status)
	}
	provider.calls++
	return &sms.SendResponse{
		ProviderID:    provider.ID(),
		ProviderMsgID: "sms-message-e2e",
		Status:        sms.StatusSubmitted,
	}, nil
}

func (provider *checkingSMSProvider) CheckStatus(context.Context, string) (*sms.StatusResponse, error) {
	return nil, errors.New("status polling is not used by this test provider")
}

func TestFreshDatabaseEmailDeliveryAndFeedback(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()
	messageID := uuid.New()

	recipients, err := json.Marshal(map[string]any{
		"to":       []map[string]string{{"email": "recipient@example.com", "name": "Recipient"}},
		"cc":       []any{},
		"bcc":      []any{},
		"reply_to": []any{},
	})
	if err != nil {
		t.Fatalf("encode email recipients: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_messages (
			id, team_id, sender_provider_binding_id, delivery_provider, provider_region,
			from_email, to_email, subject, text_body, recipients, status
		)
		VALUES ($1, $2, $3, 'aws_ses', 'us-east-1',
			'sender@example.com', 'recipient@example.com', 'Fresh database email', 'hello', $4, 'queued')
	`, messageID, fixture.TeamID, fixture.EmailBindingID, recipients); err != nil {
		t.Fatalf("seed email message: %v", err)
	}

	repository := emaildelivery.NewRepository(pool)
	claimed, err := repository.Claim(ctx, messageID, fixture.TeamID)
	if err != nil {
		t.Fatalf("claim email message: %v", err)
	}
	if claimed.AttemptID == uuid.Nil {
		t.Fatal("email claim returned an empty attempt ID")
	}
	if got := requireAttemptStatus(ctx, pool, claimed.AttemptID); got != string(platformdelivery.StatusClaimed) {
		t.Fatalf("expected claimed email attempt, got %s", got)
	}
	if err := repository.MarkRequestStarted(ctx, messageID, fixture.TeamID, claimed.AttemptID); err != nil {
		t.Fatalf("mark email request started: %v", err)
	}

	provider := &checkingEmailSender{pool: pool, attemptID: claimed.AttemptID}
	result, err := provider.Send(ctx, awsses.Message{AttemptID: claimed.AttemptID.String()})
	if err != nil {
		t.Fatalf("send email through fake provider: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one email provider call, got %d", provider.calls)
	}
	if err := repository.MarkSubmitted(ctx, messageID, fixture.TeamID, claimed.AttemptID, result); err != nil {
		t.Fatalf("mark email submitted: %v", err)
	}

	emitter := platformwebhook.NewEmitter(webhooks.NewRepository(pool))
	feedbackRepository := emailfeedback.NewRepositoryWithWebhookEmitter(pool, emitter)
	occurredAt := time.Now().UTC()
	providerPayload, err := json.Marshal(map[string]any{
		"eventType": "Delivery",
		"mail": map[string]any{
			"timestamp":   occurredAt,
			"messageId":   result.MessageID,
			"destination": []string{"recipient@example.com"},
			"tags": map[string][]string{
				"dugble_message_id": {messageID.String()},
				"dugble_attempt_id": {claimed.AttemptID.String()},
			},
		},
		"delivery": map[string]any{
			"timestamp":  occurredAt,
			"recipients": []string{"recipient@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("encode SES delivery event: %v", err)
	}
	envelope := awssns.Envelope{
		Type:      awssns.TypeNotification,
		MessageID: "sns-delivery-e2e",
		TopicARN:  "arn:aws:sns:us-east-1:123456789012:ses-feedback",
		Message:   string(providerPayload),
	}
	if err := feedbackRepository.IngestSNS(ctx, envelope); err != nil {
		t.Fatalf("ingest SES delivery event: %v", err)
	}
	var eventID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT id FROM email_provider_events
		WHERE provider_notification_id = $1
	`, envelope.MessageID).Scan(&eventID); err != nil {
		t.Fatalf("load durable email provider event: %v", err)
	}
	if err := feedbackRepository.Process(ctx, eventID); err != nil {
		t.Fatalf("process SES delivery event: %v", err)
	}
	if err := feedbackRepository.Process(ctx, eventID); err != nil {
		t.Fatalf("reprocess SES delivery event: %v", err)
	}

	if got := requireAttemptStatus(ctx, pool, claimed.AttemptID); got != string(platformdelivery.StatusDelivered) {
		t.Fatalf("expected delivered email attempt, got %s", got)
	}
	if got := requireString(t, pool, "SELECT status FROM email_messages WHERE id = $1", messageID); got != "delivered" {
		t.Fatalf("expected delivered email message, got %s", got)
	}
	if got := requireString(t, pool, `
		SELECT status FROM email_recipients
		WHERE email_message_id = $1 AND recipient_email = 'recipient@example.com'
	`, messageID); got != "delivered" {
		t.Fatalf("expected delivered email recipient, got %s", got)
	}
	requireExactlyOnceWebhook(t, pool, fixture.TeamID, "email.delivered")
	if got := requireCount(t, pool, "SELECT count(*) FROM processed_events"); got != 1 {
		t.Fatalf("expected one normalized email feedback dedupe record, got %d", got)
	}
}

func TestFreshDatabaseSMSDeliveryAndFeedback(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()
	messageID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO sms_messages (
			id, team_id, sender_provider_binding_id, to_number, from_name,
			body, status, destination_country
		)
		VALUES ($1, $2, $3, '+233200000001', 'DUGBLE', 'fresh database SMS', 'queued', 'GH')
	`, messageID, fixture.TeamID, fixture.SMSBindingID); err != nil {
		t.Fatalf("seed SMS message: %v", err)
	}

	emitter := platformwebhook.NewEmitter(webhooks.NewRepository(pool))
	messageRepository := smsmodule.NewRepositoryWithWebhookEmitter(pool, emitter)
	if _, err := messageRepository.MarkProcessing(ctx, messageID, fixture.TeamID); err != nil {
		t.Fatalf("mark SMS processing: %v", err)
	}
	routes, err := messageRepository.ResolveDeliveryRoutes(ctx, messageID, fixture.TeamID)
	if err != nil {
		t.Fatalf("resolve SMS delivery routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected one SMS route, got %d", len(routes))
	}
	attemptID, err := messageRepository.CreateDeliveryAttempt(ctx, messageID, fixture.TeamID, routes[0])
	if err != nil {
		t.Fatalf("create SMS delivery attempt: %v", err)
	}
	if got := requireAttemptStatus(ctx, pool, attemptID); got != string(platformdelivery.StatusClaimed) {
		t.Fatalf("expected claimed SMS attempt, got %s", got)
	}
	if err := messageRepository.MarkDeliveryAttemptStarted(ctx, messageID, fixture.TeamID, attemptID); err != nil {
		t.Fatalf("mark SMS request started: %v", err)
	}

	provider := &checkingSMSProvider{pool: pool, attemptID: attemptID}
	response, err := provider.Send(ctx, sms.SendRequest{
		Reference:          messageID.String(),
		To:                 "+233200000001",
		From:               "DUGBLE",
		Message:            "fresh database SMS",
		DestinationCountry: "GH",
	})
	if err != nil {
		t.Fatalf("send SMS through fake provider: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one SMS provider call, got %d", provider.calls)
	}
	if err := messageRepository.MarkDeliveryAttemptSubmitted(
		ctx,
		messageID,
		fixture.TeamID,
		attemptID,
		response,
	); err != nil {
		t.Fatalf("mark SMS submitted: %v", err)
	}

	feedbackRepository := smsfeedback.NewRepositoryWithMessageRepository(pool, messageRepository)
	occurredAt := time.Now().UTC()
	event := platformfeedback.Event{
		AttemptID:         attemptID,
		Provider:          provider.ID(),
		ProviderEventID:   "poll:" + attemptID.String() + ":1:delivered",
		ProviderMessageID: response.ProviderMsgID,
		EventType:         "sms.status.delivered",
		Channel:           messaging.ChannelSMS,
		Status:            platformdelivery.StatusDelivered,
		ProviderStatus:    sms.StatusDelivered,
		OccurredAt:        occurredAt,
		ReceivedAt:        occurredAt.Add(time.Second),
		Metadata:          json.RawMessage(`{"source":"fresh-db-e2e"}`),
	}
	first, err := feedbackRepository.Apply(ctx, event)
	if err != nil {
		t.Fatalf("apply SMS delivered feedback: %v", err)
	}
	if !first.Transitioned || first.Duplicate {
		t.Fatalf("expected first SMS feedback to transition once: %+v", first)
	}
	second, err := feedbackRepository.Apply(ctx, event)
	if err != nil {
		t.Fatalf("reapply SMS delivered feedback: %v", err)
	}
	if !second.Duplicate {
		t.Fatalf("expected duplicate SMS feedback result: %+v", second)
	}

	if got := requireAttemptStatus(ctx, pool, attemptID); got != string(platformdelivery.StatusDelivered) {
		t.Fatalf("expected delivered SMS attempt, got %s", got)
	}
	if got := requireString(t, pool, "SELECT status FROM sms_messages WHERE id = $1", messageID); got != "delivered" {
		t.Fatalf("expected delivered SMS message, got %s", got)
	}
	requireExactlyOnceWebhook(t, pool, fixture.TeamID, "sms.delivered")
	if got := requireCount(t, pool, "SELECT count(*) FROM processed_events"); got != 1 {
		t.Fatalf("expected one normalized SMS feedback dedupe record, got %d", got)
	}
}

func requireAttemptStatus(ctx context.Context, pool *pgxpool.Pool, attemptID uuid.UUID) string {
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM message_delivery_attempts WHERE id = $1
	`, attemptID).Scan(&status); err != nil {
		return "query-error:" + err.Error()
	}
	return status
}

func requireExactlyOnceWebhook(t *testing.T, pool *pgxpool.Pool, teamID uuid.UUID, eventType string) {
	t.Helper()
	if got := requireCount(t, pool, `
		SELECT count(*) FROM webhook_events
		WHERE team_id = $1 AND event_type = $2
	`, teamID, eventType); got != 1 {
		t.Fatalf("expected one %s webhook event, got %d", eventType, got)
	}
	if got := requireCount(t, pool, `
		SELECT count(*)
		FROM webhook_deliveries AS delivery
		JOIN webhook_events AS event ON event.id = delivery.event_id
		WHERE event.team_id = $1 AND event.event_type = $2
	`, teamID, eventType); got != 1 {
		t.Fatalf("expected one %s webhook delivery, got %d", eventType, got)
	}
}
