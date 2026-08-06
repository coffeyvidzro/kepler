package messaging_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type messagingFixture struct {
	TeamID          uuid.UUID
	EmailAssetID    uuid.UUID
	EmailBindingID  uuid.UUID
	EmailGrantID    uuid.UUID
	EmailTenantID   uuid.UUID
	SMSAssetID      uuid.UUID
	SMSBindingID    uuid.UUID
	SMSGrantID      uuid.UUID
	WebhookEndpoint uuid.UUID
}

func openFreshDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("reset integration database: %v", err)
	}

	root := repositoryRoot(t)
	migrationPaths, err := filepath.Glob(filepath.Join(root, "migrations", "[0-9][0-9][0-9]_*.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(migrationPaths)
	if len(migrationPaths) != 24 {
		t.Fatalf("expected 24 migrations, found %d", len(migrationPaths))
	}
	for index, migrationPath := range migrationPaths {
		expectedPrefix := fmt.Sprintf("%03d_", index+1)
		if name := filepath.Base(migrationPath); len(name) < len(expectedPrefix) || name[:len(expectedPrefix)] != expectedPrefix {
			t.Fatalf("migration sequence is not contiguous at %q; expected prefix %q", name, expectedPrefix)
		}
		contents, readErr := os.ReadFile(migrationPath)
		if readErr != nil {
			t.Fatalf("read migration %s: %v", filepath.Base(migrationPath), readErr)
		}
		if _, execErr := pool.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(migrationPath), execErr)
		}
	}

	return pool
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func seedMessagingFixture(t *testing.T, pool *pgxpool.Pool) messagingFixture {
	t.Helper()

	fixture := messagingFixture{
		TeamID:          uuid.New(),
		EmailAssetID:    uuid.New(),
		EmailBindingID:  uuid.New(),
		EmailGrantID:    uuid.New(),
		EmailTenantID:   uuid.New(),
		SMSAssetID:      uuid.New(),
		SMSBindingID:    uuid.New(),
		SMSGrantID:      uuid.New(),
		WebhookEndpoint: uuid.New(),
	}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO teams (id, name, market_code, phone, address, status)
		VALUES ($1, 'Messaging E2E', 'GH', '+233200000000', 'Accra', 'active')
	`, fixture.TeamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_assets (
			id, owner_type, team_id, channel, identity, normalized_identity,
			status, health_status
		)
		VALUES
			($1, 'team', $2, 'email', 'sender@example.com', 'sender@example.com', 'active', 'healthy'),
			($3, 'team', $2, 'sms', 'DUGBLE', 'dugble', 'active', 'healthy')
	`, fixture.EmailAssetID, fixture.TeamID, fixture.SMSAssetID); err != nil {
		t.Fatalf("seed sender assets: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_provider_bindings (
			id, sender_asset_id, provider, provider_account, region, country_code,
			status, verified, provider_whitelisted, health_status, verified_at
		)
		VALUES
			($1, $2, 'ses', 'default', 'us-east-1', NULL, 'active', true, true, 'healthy', now()),
			($3, $4, 'mnotify', 'default', NULL, 'GH', 'active', true, true, 'healthy', now())
	`, fixture.EmailBindingID, fixture.EmailAssetID, fixture.SMSBindingID, fixture.SMSAssetID); err != nil {
		t.Fatalf("seed provider bindings: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sender_asset_grants (
			id, team_id, sender_asset_id, channel, status, is_default
		)
		VALUES
			($1, $2, $3, 'email', 'active', true),
			($4, $2, $5, 'sms', 'active', true)
	`, fixture.EmailGrantID, fixture.TeamID, fixture.EmailAssetID, fixture.SMSGrantID, fixture.SMSAssetID); err != nil {
		t.Fatalf("seed sender grants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_tenants (
			id, team_id, provider, region, external_name, external_id,
			tenant_arn, status, suppression_scope, reputation_policy
		)
		VALUES (
			$1, $2, 'aws_ses', 'us-east-1', 'messaging-e2e', 'tenant-messaging-e2e',
			'arn:aws:ses:us-east-1:123456789012:tenant/messaging-e2e',
			'active', 'tenant', 'standard'
		)
	`, fixture.EmailTenantID, fixture.TeamID); err != nil {
		t.Fatalf("seed email tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (
			id, team_id, url, signing_secret, enabled, subscribed_events
		)
		VALUES ($1, $2, 'https://example.test/messaging-events', decode('736563726574', 'hex'), true,
			ARRAY['email.delivered', 'sms.delivered']::text[])
	`, fixture.WebhookEndpoint, fixture.TeamID); err != nil {
		t.Fatalf("seed webhook endpoint: %v", err)
	}

	return fixture
}

func requireString(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query string value: %v", err)
	}
	return value
}

func requireCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var value int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query count: %v", err)
	}
	return value
}
