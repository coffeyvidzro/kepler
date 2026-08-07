from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"anchor not found in {path}")
    file.write_text(text.replace(old, new, 1))


replace_once(
    "internal/modules/contact/model.go",
    "type CreateRequest struct {\n",
    '''type SegmentMembership struct {
\tID         string    `json:"id"`
\tTeamID     string    `json:"team_id"`
\tName       string    `json:"name"`
\tCreatedAt  time.Time `json:"created_at"`
\tAssignedAt time.Time `json:"assigned_at"`
}

type CreateRequest struct {
''',
)

replace_once(
    "internal/modules/contact/repository.go",
    '''\tErrPropertyTypeMismatch = errors.New("contact property type mismatch")
''',
    '''\tErrPropertyTypeMismatch = errors.New("contact property type mismatch")
\tErrContactNotFound      = errors.New("contact not found")
\tErrSegmentNotFound      = errors.New("segment not found")
''',
)

repository_methods = r'''func (r *Repository) ListSegments(ctx context.Context, contactID, teamID uuid.UUID) ([]SegmentMembership, error) {
	if err := ensureContactExists(ctx, r.db, contactID, teamID, false); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT segment.id, segment.team_id, segment.name, segment.created_at, membership.created_at
		FROM contact_segments AS membership
		JOIN segments AS segment
		  ON segment.id = membership.segment_id
		 AND segment.team_id = membership.team_id
		WHERE membership.team_id = $1
		  AND membership.contact_id = $2
		ORDER BY membership.created_at DESC, segment.id DESC
	`, teamID, contactID)
	if err != nil {
		return nil, fmt.Errorf("list contact segments: %w", err)
	}
	defer rows.Close()

	memberships := make([]SegmentMembership, 0)
	for rows.Next() {
		var membership SegmentMembership
		if err := rows.Scan(&membership.ID, &membership.TeamID, &membership.Name, &membership.CreatedAt, &membership.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan contact segment membership: %w", err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact segment memberships: %w", err)
	}
	return memberships, nil
}

func (r *Repository) AddSegment(ctx context.Context, contactID, segmentID, teamID uuid.UUID) (SegmentMembership, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SegmentMembership{}, false, fmt.Errorf("begin contact segment assignment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := ensureContactExists(ctx, tx, contactID, teamID, true); err != nil {
		return SegmentMembership{}, false, err
	}
	membership, err := getMembershipSegment(ctx, tx, segmentID, teamID, true)
	if err != nil {
		return SegmentMembership{}, false, err
	}

	created := true
	err = tx.QueryRow(ctx, `
		INSERT INTO contact_segments (team_id, contact_id, segment_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (contact_id, segment_id) DO NOTHING
		RETURNING created_at
	`, teamID, contactID, segmentID).Scan(&membership.AssignedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		created = false
		err = tx.QueryRow(ctx, `
			SELECT created_at FROM contact_segments
			WHERE team_id = $1 AND contact_id = $2 AND segment_id = $3
		`, teamID, contactID, segmentID).Scan(&membership.AssignedAt)
	}
	if err != nil {
		return SegmentMembership{}, false, fmt.Errorf("assign contact to segment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SegmentMembership{}, false, fmt.Errorf("commit contact segment assignment: %w", err)
	}
	return membership, created, nil
}

func (r *Repository) RemoveSegment(ctx context.Context, contactID, segmentID, teamID uuid.UUID) (bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin contact segment removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := ensureContactExists(ctx, tx, contactID, teamID, true); err != nil {
		return false, err
	}
	if _, err := getMembershipSegment(ctx, tx, segmentID, teamID, true); err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `
		DELETE FROM contact_segments
		WHERE team_id = $1 AND contact_id = $2 AND segment_id = $3
	`, teamID, contactID, segmentID)
	if err != nil {
		return false, fmt.Errorf("remove contact from segment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit contact segment removal: %w", err)
	}
	return command.RowsAffected() > 0, nil
}

func ensureContactExists(ctx context.Context, db contactQueryer, contactID, teamID uuid.UUID, lock bool) error {
	query := `SELECT 1 FROM contacts WHERE id = $1 AND team_id = $2`
	if lock {
		query += ` FOR SHARE`
	}
	var exists int
	err := db.QueryRow(ctx, query, contactID, teamID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrContactNotFound
	}
	if err != nil {
		return fmt.Errorf("validate contact for segment membership: %w", err)
	}
	return nil
}

func getMembershipSegment(ctx context.Context, db contactQueryer, segmentID, teamID uuid.UUID, lock bool) (SegmentMembership, error) {
	query := `SELECT id, team_id, name, created_at FROM segments WHERE id = $1 AND team_id = $2`
	if lock {
		query += ` FOR SHARE`
	}
	var membership SegmentMembership
	err := db.QueryRow(ctx, query, segmentID, teamID).Scan(&membership.ID, &membership.TeamID, &membership.Name, &membership.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SegmentMembership{}, ErrSegmentNotFound
	}
	if err != nil {
		return SegmentMembership{}, fmt.Errorf("validate segment for contact membership: %w", err)
	}
	return membership, nil
}

'''
replace_once(
    "internal/modules/contact/repository.go",
    "type contactQueryer interface {\n",
    repository_methods + "type contactQueryer interface {\n",
)

service_methods = r'''func (s *Service) ListSegments(ctx context.Context, contactValue string) ([]SegmentMembership, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsRead)
	if err != nil {
		return nil, err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return nil, err
	}
	memberships, err := s.repository.ListSegments(ctx, contactID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return nil, apperrors.NewNotFound("Contact not found")
	}
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list contact segments", err)
	}
	return memberships, nil
}

func (s *Service) AddSegment(ctx context.Context, contactValue, segmentValue string) (SegmentMembership, bool, error) {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return SegmentMembership{}, false, err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return SegmentMembership{}, false, err
	}
	segmentID, err := parseID(segmentValue, "Segment")
	if err != nil {
		return SegmentMembership{}, false, err
	}
	membership, created, err := s.repository.AddSegment(ctx, contactID, segmentID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return SegmentMembership{}, false, apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrSegmentNotFound) {
		return SegmentMembership{}, false, apperrors.NewNotFound("Segment not found")
	}
	if err != nil {
		return SegmentMembership{}, false, apperrors.NewInternal("Unable to add contact to segment", err)
	}
	if created {
		audit.Record(ctx, access, audit.Event{Action: "contact.segment_added", ResourceType: "contact", ResourceID: contactID.String()})
	}
	return membership, created, nil
}

func (s *Service) RemoveSegment(ctx context.Context, contactValue, segmentValue string) error {
	access, err := requireTenant(ctx, tenant.PermissionContactsWrite)
	if err != nil {
		return err
	}
	contactID, err := parseID(contactValue, "Contact")
	if err != nil {
		return err
	}
	segmentID, err := parseID(segmentValue, "Segment")
	if err != nil {
		return err
	}
	removed, err := s.repository.RemoveSegment(ctx, contactID, segmentID, access.Scope.TeamID)
	if errors.Is(err, ErrContactNotFound) {
		return apperrors.NewNotFound("Contact not found")
	}
	if errors.Is(err, ErrSegmentNotFound) {
		return apperrors.NewNotFound("Segment not found")
	}
	if err != nil {
		return apperrors.NewInternal("Unable to remove contact from segment", err)
	}
	if removed {
		audit.Record(ctx, access, audit.Event{Action: "contact.segment_removed", ResourceType: "contact", ResourceID: contactID.String()})
	}
	return nil
}

'''
replace_once(
    "internal/modules/contact/service.go",
    "func validateCreate(req CreateRequest) (CreateRequest, error) {\n",
    service_methods + "func validateCreate(req CreateRequest) (CreateRequest, error) {\n",
)

handler_methods = r'''func (h *Handler) ListSegments(c *echo.Context) error {
	value, err := h.service.ListSegments(c.Request().Context(), c.Param("contact_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, value)
}

func (h *Handler) AddSegment(c *echo.Context) error {
	value, created, err := h.service.AddSegment(c.Request().Context(), c.Param("contact_id"), c.Param("segment_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	if created {
		return httputil.Created(c, value)
	}
	return httputil.OK(c, value)
}

func (h *Handler) RemoveSegment(c *echo.Context) error {
	if err := h.service.RemoveSegment(c.Request().Context(), c.Param("contact_id"), c.Param("segment_id")); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.NoContent(c)
}

'''
replace_once(
    "internal/transport/http/contact/handler.go",
    "func decodeJSON(c *echo.Context, dst any) error {\n",
    handler_methods + "func decodeJSON(c *echo.Context, dst any) error {\n",
)

replace_once(
    "internal/transport/http/contact/routes.go",
    '''\tcontacts.GET("", handler.List, accessMiddleware(tenant.PermissionContactsRead))
\tcontacts.GET("/:contact_id", handler.Get, accessMiddleware(tenant.PermissionContactsRead))
''',
    '''\tcontacts.GET("", handler.List, accessMiddleware(tenant.PermissionContactsRead))
\tcontacts.GET("/:contact_id/segments", handler.ListSegments, accessMiddleware(tenant.PermissionContactsRead))
\tcontacts.POST("/:contact_id/segments/:segment_id", handler.AddSegment, accessMiddleware(tenant.PermissionContactsWrite))
\tcontacts.DELETE("/:contact_id/segments/:segment_id", handler.RemoveSegment, accessMiddleware(tenant.PermissionContactsWrite))
\tcontacts.GET("/:contact_id", handler.Get, accessMiddleware(tenant.PermissionContactsRead))
''',
)

Path("internal/integration/messaging/contact_segment_memberships_test.go").write_text(r'''package messaging_test

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
''')
