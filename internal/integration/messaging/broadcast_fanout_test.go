package messaging_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	broadcastexecution "github.com/coffeyvidzro/dugble/server/internal/delivery/broadcast"
	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	messagetemplatemodule "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

type broadcastFanoutQueue struct{ enqueued int }

func (queue *broadcastFanoutQueue) EnqueueEmailDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error {
	queue.enqueued++
	return nil
}

type broadcastFanoutBilling struct {
	charged  int
	observed int
}

func (billing *broadcastFanoutBilling) ChargeEmail(context.Context, pgx.Tx, platformbilling.EmailChargeInput) (platformbilling.Charge, error) {
	billing.charged++
	return platformbilling.Charge{}, nil
}

func (billing *broadcastFanoutBilling) ObserveCommittedCharge(context.Context, platformbilling.CommittedCharge) {
	billing.observed++
}

func TestBroadcastFanoutPersistsEmailAndFinalizesBroadcast(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()

	segmentID := uuid.New()
	templateID := uuid.New()
	versionID := uuid.New()
	broadcastID := uuid.New()
	recipientID := uuid.New()
	domainAssetID := uuid.New()
	domainBindingID := uuid.New()
	domainGrantID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_assets (
			id, owner_type, team_id, channel, identity, normalized_identity,
			status, health_status
		) VALUES ($1, 'team', $2, 'email', 'example.com', 'example.com', 'active', 'healthy')
	`, domainAssetID, fixture.TeamID); err != nil {
		t.Fatalf("seed broadcast sender domain asset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_provider_bindings (
			id, sender_asset_id, provider, provider_account, region,
			status, verified, provider_whitelisted, health_status, verified_at
		) VALUES ($1, $2, 'ses', 'default', 'us-east-1', 'active', true, true, 'healthy', now())
	`, domainBindingID, domainAssetID); err != nil {
		t.Fatalf("seed broadcast sender domain binding: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_asset_grants (
			id, team_id, sender_asset_id, channel, status, is_default
		) VALUES ($1, $2, $3, 'email', 'active', false)
	`, domainGrantID, fixture.TeamID, domainAssetID); err != nil {
		t.Fatalf("seed broadcast sender domain grant: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO segments (id, team_id, name) VALUES ($1, $2, 'Broadcast Fanout Segment')`, segmentID, fixture.TeamID); err != nil {
		t.Fatalf("seed broadcast segment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO message_templates (id, team_id, name, alias, next_version_number)
		VALUES ($1, $2, 'Broadcast Fanout Template', 'broadcast_fanout', 2)
	`, templateID, fixture.TeamID); err != nil {
		t.Fatalf("seed broadcast template: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO message_template_versions (
			id, team_id, template_id, version_number, from_email, from_name,
			subject, html_body, text_body, variables
		) VALUES (
			$1, $2, $3, 1, 'sender@example.com', 'Dugble',
			'Hello {{{ plan }}}', '<p>Hello {{{ plan }}}</p>', 'Hello {{{ plan }}}',
			'[{"key":"plan","type":"string"}]'::jsonb
		)
	`, versionID, fixture.TeamID, templateID); err != nil {
		t.Fatalf("seed broadcast template version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE message_templates
		SET current_version_id = $1, published_version_id = $1, published_at = now()
		WHERE id = $2 AND team_id = $3
	`, versionID, templateID, fixture.TeamID); err != nil {
		t.Fatalf("publish broadcast template version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO broadcasts (
			id, team_id, name, status, segment_id, template_id, template_version_id,
			variable_bindings, queued_at, recipients_materialized_at,
			audience_count, eligible_count
		) VALUES (
			$1, $2, 'Broadcast Fanout', 'queued', $3, $4, $5,
			'{"plan":"enterprise"}'::jsonb, now(), now(), 1, 1
		)
	`, broadcastID, fixture.TeamID, segmentID, templateID, versionID); err != nil {
		t.Fatalf("seed queued broadcast: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO broadcast_recipients (
			id, team_id, broadcast_id, email, normalized_email,
			first_name, last_name, contact_snapshot, status
		) VALUES (
			$1, $2, $3, 'ada@example.com', 'ada@example.com',
			'Ada', 'Lovelace', '{"properties":{"plan":"pro"}}'::jsonb, 'pending'
		)
	`, recipientID, fixture.TeamID, broadcastID); err != nil {
		t.Fatalf("seed pending broadcast recipient: %v", err)
	}

	queue := &broadcastFanoutQueue{}
	billing := &broadcastFanoutBilling{}
	emailService := emailmodule.NewService(
		emailmodule.NewRepository(pool),
		queue,
		emailmodule.ServiceConfig{
			DefaultFromEmail: "sender@example.com",
			DefaultProvider:  domainmodule.DefaultProvider,
			DefaultRegion:    "us-east-1",
		},
		billing,
	)
	templateService := messagetemplatemodule.NewService(messagetemplatemodule.NewRepository(pool), emailService)
	processor := broadcastexecution.NewProcessor(broadcastmodule.NewRepository(pool), templateService, emailService)

	if err := processor.ProcessBatch(ctx, 1); err != nil {
		t.Fatalf("process broadcast fanout: %v", err)
	}

	var recipientStatus string
	var emailMessageID uuid.UUID
	var attemptCount int32
	if err := pool.QueryRow(ctx, `
		SELECT status, email_message_id, attempt_count
		FROM broadcast_recipients WHERE id = $1
	`, recipientID).Scan(&recipientStatus, &emailMessageID, &attemptCount); err != nil {
		t.Fatalf("load fanned-out recipient: %v", err)
	}
	if recipientStatus != "queued" {
		t.Fatalf("recipient status = %q, want queued", recipientStatus)
	}
	if attemptCount != 0 {
		t.Fatalf("recipient attempt_count = %d, want 0", attemptCount)
	}

	var subject, toEmail, messageType string
	if err := pool.QueryRow(ctx, `
		SELECT subject, to_email, message_type
		FROM email_messages WHERE id = $1 AND team_id = $2
	`, emailMessageID, fixture.TeamID).Scan(&subject, &toEmail, &messageType); err != nil {
		t.Fatalf("load generated broadcast email: %v", err)
	}
	if subject != "Hello enterprise" {
		t.Fatalf("email subject = %q, want %q", subject, "Hello enterprise")
	}
	if toEmail != "ada@example.com" {
		t.Fatalf("email to = %q, want ada@example.com", toEmail)
	}
	if messageType != emailmodule.MessageTypeMarketing {
		t.Fatalf("email message_type = %q, want marketing", messageType)
	}

	var broadcastStatus string
	var queuedCount, failedCount int64
	if err := pool.QueryRow(ctx, `
		SELECT status, queued_count, failed_count
		FROM broadcasts WHERE id = $1 AND team_id = $2
	`, broadcastID, fixture.TeamID).Scan(&broadcastStatus, &queuedCount, &failedCount); err != nil {
		t.Fatalf("load finalized broadcast: %v", err)
	}
	if broadcastStatus != broadcastmodule.StatusSent {
		t.Fatalf("broadcast status = %q, want sent", broadcastStatus)
	}
	if queuedCount != 1 || failedCount != 0 {
		t.Fatalf("broadcast counts queued=%d failed=%d, want queued=1 failed=0", queuedCount, failedCount)
	}
	if queue.enqueued != 1 {
		t.Fatalf("delivery enqueue calls = %d, want 1", queue.enqueued)
	}
	if billing.charged != 1 || billing.observed != 1 {
		t.Fatalf("billing charged=%d observed=%d, want 1 and 1", billing.charged, billing.observed)
	}
}
