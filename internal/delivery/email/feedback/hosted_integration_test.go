//go:build integration

package feedback

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	awssns "github.com/coffeyvidzro/dugble/server/internal/integration/aws/sns"
	jetstreammessaging "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/processed"
)

func TestHostedPostgresJetStreamFeedbackFlow(t *testing.T) {
	db, client := hostedDependencies(t)
	resetHostedFeedbackData(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageID := uuid.New()
	teamID := uuid.New()
	providerMessageID := "ses-" + uuid.NewString()
	insertHostedMessage(t, db, teamID, messageID, providerMessageID, "submitted")

	metrics := NewMetrics()
	repository := NewRepository(db, outbox.NewRepository(db))
	notificationID := uuid.NewString()
	if err := repository.Ingest(ctx, awssns.Envelope{
		Type:      awssns.TypeNotification,
		MessageID: notificationID,
		TopicARN:  "arn:aws:sns:us-east-1:123456789012:dugble-feedback",
		Message:   hostedBouncePayload(providerMessageID, messageID, false),
	}); err != nil {
		t.Fatalf("ingest hosted SES event: %v", err)
	}

	consumer := NewConsumer(
		client,
		processed.NewRepository(db),
		NewHandlerWithMetrics(NewRepository(db, nil), metrics),
		ConsumerConfig{
			Concurrency:    1,
			AckWait:        2 * time.Second,
			HandlerTimeout: 2 * time.Second,
			MaxDeliver:     3,
			RetryPolicy:    RetryPolicy{Delays: []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, time.Second}},
		},
	)
	relay := outbox.NewRelay(outbox.NewRepository(db), client, outbox.Config{
		PollInterval: 25 * time.Millisecond,
		BatchSize:    10,
		LockTimeout:  time.Second,
	})

	runComponent(t, ctx, "feedback consumer", consumer.Run)
	runComponent(t, ctx, "outbox relay", relay.Run)

	eventID := uuid.NewSHA1(eventNamespace, []byte("arn:aws:sns:us-east-1:123456789012:dugble-feedback:"+notificationID))
	awaitHosted(t, 10*time.Second, func() (bool, error) {
		var processed bool
		var messageStatus, recipientStatus, statusCode, diagnosticCode string
		err := db.QueryRow(context.Background(), `
			SELECT
				provider_event.processed_at IS NOT NULL,
				message.status,
				recipient.status,
				COALESCE(recipient.last_status_code, ''),
				COALESCE(recipient.last_diagnostic_code, '')
			FROM email_provider_events AS provider_event
			JOIN email_messages AS message ON message.id = provider_event.email_message_id
			JOIN email_recipients AS recipient ON recipient.email_message_id = message.id
			WHERE provider_event.id = $1
			  AND recipient.recipient_email = 'recipient@example.com'
		`, eventID).Scan(&processed, &messageStatus, &recipientStatus, &statusCode, &diagnosticCode)
		if err != nil {
			return false, err
		}
		return processed && messageStatus == "bounced" && recipientStatus == "bounced" && statusCode == "5.1.1" && strings.Contains(diagnosticCode, "550"), nil
	})

	collector := NewMetricsCollector(db, metrics, time.Second)
	if err := collector.collect(context.Background()); err != nil {
		t.Fatalf("collect feedback metrics: %v", err)
	}
	recorder := httptest.NewRecorder()
	metrics.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, expected := range []string{
		`dugble_email_feedback_events_total{stage="process",event_type="provider_event",outcome="success"} 1`,
		`dugble_email_feedback_reconciliation_queue_events{state="scheduled"} 0`,
		`dugble_email_feedback_operation_duration_seconds_count{operation="process"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics output missing %q:\n%s", expected, body)
		}
	}
}

func TestHostedPostgresReconcilesInitiallyUnlinkedEvent(t *testing.T) {
	db, _ := hostedDependencies(t)
	resetHostedFeedbackData(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageID := uuid.New()
	teamID := uuid.New()
	providerMessageID := "ses-" + uuid.NewString()
	insertHostedMessage(t, db, teamID, messageID, "", "submission_unknown")

	repository := NewRepository(db, outbox.NewRepository(db))
	notificationID := uuid.NewString()
	if err := repository.Ingest(ctx, awssns.Envelope{
		Type:      awssns.TypeNotification,
		MessageID: notificationID,
		TopicARN:  "arn:aws:sns:us-east-1:123456789012:dugble-feedback",
		Message:   hostedBouncePayload(providerMessageID, messageID, true),
	}); err != nil {
		t.Fatalf("ingest unlinked SES event: %v", err)
	}

	metrics := NewMetrics()
	reconciler := NewObservedReconciler(NewRepository(db, nil), ReconcilerConfig{
		PollInterval:  25 * time.Millisecond,
		BatchSize:     10,
		Concurrency:   1,
		LeaseDuration: time.Second,
		HandleTimeout: 2 * time.Second,
	}, metrics)
	runComponent(t, ctx, "database reconciler", reconciler.Run)

	eventID := uuid.NewSHA1(eventNamespace, []byte("arn:aws:sns:us-east-1:123456789012:dugble-feedback:"+notificationID))
	awaitHosted(t, 10*time.Second, func() (bool, error) {
		var linkedMessageID *uuid.UUID
		var processed bool
		var attempts int
		var status, storedProviderMessageID string
		err := db.QueryRow(context.Background(), `
			SELECT
				provider_event.email_message_id,
				provider_event.processed_at IS NOT NULL,
				provider_event.attempt_count,
				message.status,
				COALESCE(message.provider_message_id, '')
			FROM email_provider_events AS provider_event
			JOIN email_messages AS message ON message.id = $2
			WHERE provider_event.id = $1
		`, eventID, messageID).Scan(&linkedMessageID, &processed, &attempts, &status, &storedProviderMessageID)
		if err != nil {
			return false, err
		}
		return linkedMessageID != nil && *linkedMessageID == messageID && processed && attempts >= 1 && status == "bounced" && storedProviderMessageID == providerMessageID, nil
	})

	recorder := httptest.NewRecorder()
	metrics.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(recorder.Body.String(), `dugble_email_feedback_events_total{stage="reconcile",event_type="provider_event",outcome="processed"} 1`) {
		t.Fatalf("reconciliation metric missing:\n%s", recorder.Body.String())
	}
}

func hostedDependencies(t *testing.T) (*pgxpool.Pool, *jetstreammessaging.Client) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("INTEGRATION_DATABASE_URL"))
	natsURL := strings.TrimSpace(os.Getenv("INTEGRATION_NATS_URL"))
	if databaseURL == "" || natsURL == "" {
		t.Skip("hosted integration services are not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect hosted PostgreSQL: %v", err)
	}
	t.Cleanup(db.Close)
	client, err := jetstreammessaging.New(ctx, natsURL, "dugble-feedback-integration")
	if err != nil {
		t.Fatalf("connect hosted JetStream: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Provision(ctx, jetstreammessaging.DefaultStreamLimits()); err != nil {
		t.Fatalf("provision hosted JetStream: %v", err)
	}
	return db, client
}

func resetHostedFeedbackData(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		TRUNCATE TABLE
			email_provider_events,
			email_messages,
			teams,
			outbox_events,
			processed_events
		RESTART IDENTITY CASCADE
	`); err != nil {
		t.Fatalf("reset hosted integration data: %v", err)
	}
}

func insertHostedMessage(t *testing.T, db *pgxpool.Pool, teamID, messageID uuid.UUID, providerMessageID, status string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `INSERT INTO teams (id, name, market_code) VALUES ($1, 'Hosted Integration', 'GH')`, teamID); err != nil {
		t.Fatalf("insert hosted team: %v", err)
	}
	var provider any
	var providerID any
	if strings.TrimSpace(providerMessageID) != "" {
		provider = ProviderSES
		providerID = providerMessageID
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO email_messages (
			id, team_id, from_email, to_email, subject, text_body, status,
			provider, provider_message_id, recipients
		)
		VALUES (
			$1, $2, 'sender@example.com', 'recipient@example.com', 'Hosted integration',
			'Hosted integration body', $3, $4, $5,
			'{"to":[{"email":"recipient@example.com"}],"cc":[],"bcc":[],"reply_to":[]}'::jsonb
		)
	`, messageID, teamID, status, provider, providerID); err != nil {
		t.Fatalf("insert hosted email message: %v", err)
	}
}

func hostedBouncePayload(providerMessageID string, messageID uuid.UUID, includeTag bool) string {
	tags := "{}"
	if includeTag {
		tags = fmt.Sprintf(`{"dugble_message_id":[%q]}`, messageID.String())
	}
	return fmt.Sprintf(`{
		"eventType":"Bounce",
		"mail":{
			"timestamp":"2026-08-01T01:00:00Z",
			"messageId":%q,
			"destination":["recipient@example.com"],
			"tags":%s
		},
		"bounce":{
			"timestamp":"2026-08-01T01:00:01Z",
			"bounceType":"Permanent",
			"bounceSubType":"General",
			"bouncedRecipients":[{
				"emailAddress":"recipient@example.com",
				"action":"failed",
				"status":"5.1.1",
				"diagnosticCode":"smtp; 550 user unknown"
			}]
		}
	}`, providerMessageID, tags)
}

func runComponent(t *testing.T, ctx context.Context, name string, run func(context.Context) error) {
	t.Helper()
	go func() {
		if err := run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("%s stopped: %v", name, err)
		}
	}()
}

func awaitHosted(t *testing.T, timeout time.Duration, condition func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := condition()
		if err == nil && ok {
			return
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("condition not met before timeout: %v", lastErr)
	}
	t.Fatal("condition not met before timeout")
}
