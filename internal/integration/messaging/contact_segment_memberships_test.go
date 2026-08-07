package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	contactmodule "github.com/coffeyvidzro/dugble/server/internal/modules/contact"
)

func TestContactSegmentMembershipLifecycle(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()
	contactID := uuid.New()
	segmentID := uuid.New()

	if _, err := pool.Exec(ctx, `INSERT INTO contacts (id, team_id, email) VALUES ($1, $2, 'ada@example.com')`, contactID, fixture.TeamID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO segments (id, team_id, name) VALUES ($1, $2, 'Customers')`, segmentID, fixture.TeamID); err != nil {
		t.Fatalf("seed segment: %v", err)
	}

	repository := contactmodule.NewRepository(pool)
	memberships, err := repository.ListSegments(ctx, contactID, fixture.TeamID)
	if err != nil || len(memberships) != 0 {
		t.Fatalf("empty memberships = %#v, err=%v", memberships, err)
	}
	membership, created, err := repository.AddSegment(ctx, contactID, segmentID, fixture.TeamID)
	if err != nil || !created {
		t.Fatalf("add membership created=%v err=%v", created, err)
	}
	if membership.ID != segmentID.String() || membership.Name != "Customers" || membership.AssignedAt.IsZero() {
		t.Fatalf("membership = %#v", membership)
	}
	repeated, created, err := repository.AddSegment(ctx, contactID, segmentID, fixture.TeamID)
	if err != nil || created || !repeated.AssignedAt.Equal(membership.AssignedAt) {
		t.Fatalf("repeat membership = %#v created=%v err=%v", repeated, created, err)
	}
	memberships, err = repository.ListSegments(ctx, contactID, fixture.TeamID)
	if err != nil || len(memberships) != 1 {
		t.Fatalf("memberships = %#v err=%v", memberships, err)
	}
	removed, err := repository.RemoveSegment(ctx, contactID, segmentID, fixture.TeamID)
	if err != nil || !removed {
		t.Fatalf("remove membership removed=%v err=%v", removed, err)
	}
	removed, err = repository.RemoveSegment(ctx, contactID, segmentID, fixture.TeamID)
	if err != nil || removed {
		t.Fatalf("repeat remove removed=%v err=%v", removed, err)
	}
}

func TestContactSegmentMembershipIsTenantSafe(t *testing.T) {
	pool := openFreshDatabase(t)
	fixture := seedMessagingFixture(t, pool)
	ctx := context.Background()
	contactID := uuid.New()
	otherTeamID := uuid.New()
	otherSegmentID := uuid.New()

	if _, err := pool.Exec(ctx, `INSERT INTO contacts (id, team_id, email) VALUES ($1, $2, 'grace@example.com')`, contactID, fixture.TeamID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO teams (id, name, market_code, phone, address, status) VALUES ($1, 'Other Team', 'GH', '+233200000001', 'Accra', 'active')`, otherTeamID); err != nil {
		t.Fatalf("seed other team: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO segments (id, team_id, name) VALUES ($1, $2, 'Other Segment')`, otherSegmentID, otherTeamID); err != nil {
		t.Fatalf("seed other segment: %v", err)
	}

	repository := contactmodule.NewRepository(pool)
	if _, _, err := repository.AddSegment(ctx, contactID, otherSegmentID, fixture.TeamID); !errors.Is(err, contactmodule.ErrSegmentNotFound) {
		t.Fatalf("cross-tenant assignment error = %v", err)
	}
	if _, err := repository.ListSegments(ctx, uuid.New(), fixture.TeamID); !errors.Is(err, contactmodule.ErrContactNotFound) {
		t.Fatalf("missing contact list error = %v", err)
	}
}
