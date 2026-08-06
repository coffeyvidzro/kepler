package messaging_test

import (
	"context"
	"errors"
	"testing"

	messagingrouting "github.com/coffeyvidzro/dugble/server/internal/delivery/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
)

func TestFreshDatabaseAppliesCanonicalMigrationsAndRoutes(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()

	for _, table := range []string{
		"sender_assets",
		"sender_provider_bindings",
		"sender_asset_grants",
		"email_messages",
		"sms_messages",
		"message_delivery_attempts",
		"processed_events",
		"webhook_events",
	} {
		if got := requireString(t, pool, "SELECT COALESCE(to_regclass($1)::text, '')", table); got != table {
			t.Fatalf("expected table %s after fresh migration, got %q", table, got)
		}
	}

	emailRoute, err := messagingrouting.Resolve(ctx, pool, platformrouting.Request{
		TeamID:               fixture.TeamID,
		Channel:              messaging.ChannelEmail,
		Provider:             "ses",
		ProviderAccount:      "default",
		DestinationRegion:    "us-east-1",
		RequiredCapabilities: []sender.Capability{sender.CapabilityDomainVerification},
	})
	if err != nil {
		t.Fatalf("resolve email route: %v", err)
	}
	if emailRoute.SenderAssetID != fixture.EmailAssetID || emailRoute.SenderProviderBindingID != fixture.EmailBindingID {
		t.Fatalf("resolved unexpected email route: %+v", emailRoute)
	}

	smsRoute, err := messagingrouting.Resolve(ctx, pool, platformrouting.Request{
		TeamID:               fixture.TeamID,
		Channel:              messaging.ChannelSMS,
		DestinationCountry:   "GH",
		RequiredCapabilities: []sender.Capability{sender.CapabilitySenderIDRegistration},
	})
	if err != nil {
		t.Fatalf("resolve SMS route: %v", err)
	}
	if smsRoute.SenderAssetID != fixture.SMSAssetID || smsRoute.SenderProviderBindingID != fixture.SMSBindingID {
		t.Fatalf("resolved unexpected SMS route: %+v", smsRoute)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE sender_asset_grants
		SET status = 'revoked', revoked_at = now(), is_default = false
		WHERE id = $1
	`, fixture.SMSGrantID); err != nil {
		t.Fatalf("revoke SMS grant: %v", err)
	}
	_, err = messagingrouting.Resolve(ctx, pool, platformrouting.Request{
		TeamID:             fixture.TeamID,
		Channel:            messaging.ChannelSMS,
		DestinationCountry: "GH",
	})
	if !errors.Is(err, platformrouting.ErrNoEligibleRoute) {
		t.Fatalf("expected revoked grant to block routing, got %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE sender_asset_grants
		SET status = 'active', revoked_at = NULL, is_default = true
		WHERE id = $1;
		UPDATE sender_provider_bindings
		SET health_status = 'degraded'
		WHERE id = $2
	`, fixture.SMSGrantID, fixture.SMSBindingID); err != nil {
		t.Fatalf("degrade SMS binding: %v", err)
	}
	_, err = messagingrouting.Resolve(ctx, pool, platformrouting.Request{
		TeamID:             fixture.TeamID,
		Channel:            messaging.ChannelSMS,
		DestinationCountry: "GH",
	})
	if !errors.Is(err, platformrouting.ErrNoEligibleRoute) {
		t.Fatalf("expected degraded binding to block routing, got %v", err)
	}
}
