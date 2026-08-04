package team

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

var (
	ErrInvitationNotAccepted   = errors.New("invitation not accepted")
	ErrTeamMemberAlreadyExists = errors.New("team member already exists")
)

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) CreateWithOwner(ctx context.Context, name, marketCode, phone, address string, website *string, ownerID uuid.UUID) (Team, error) {
	row, err := r.queries.CreateTeamWithOwner(ctx, dbsqlc.CreateTeamWithOwnerParams{
		Name: name, MarketCode: marketCode, Phone: phone, Address: address, Website: website, OwnerID: &ownerID,
	})
	if err != nil {
		return Team{}, fmt.Errorf("create team with owner: %w", err)
	}
	return Team{
		ID: row.ID.String(), Name: row.Name, MarketCode: row.MarketCode,
		Phone: row.Phone, Address: row.Address, Website: row.Website,
		Status: row.Status, CreatedBy: stringPointer(row.CreatedBy),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Team, error) {
	row, err := r.queries.GetTeam(ctx, dbsqlc.GetTeamParams{ID: id})
	if err != nil {
		return Team{}, fmt.Errorf("get team: %w", err)
	}
	return teamFromSQLC(row), nil
}

func (r *Repository) ListForUser(ctx context.Context, userID uuid.UUID) ([]Team, error) {
	rows, err := r.queries.ListTeamsForUser(ctx, dbsqlc.ListTeamsForUserParams{UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list teams for user: %w", err)
	}
	teams := make([]Team, 0, len(rows))
	for _, row := range rows {
		teams = append(teams, teamFromSQLC(row))
	}
	return teams, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, name string) (Team, error) {
	row, err := r.queries.UpdateTeam(ctx, dbsqlc.UpdateTeamParams{ID: id, Name: name})
	if err != nil {
		return Team{}, fmt.Errorf("update team: %w", err)
	}
	return teamFromSQLC(row), nil
}

func (r *Repository) Disable(ctx context.Context, id uuid.UUID) (Team, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Team{}, fmt.Errorf("begin disable team transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := r.queries.WithTx(tx).DisableTeam(ctx, dbsqlc.DisableTeamParams{ID: id})
	if err != nil {
		return Team{}, fmt.Errorf("disable team: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE webhook_deliveries AS delivery
SET status = 'canceled', last_error = 'Team disabled before webhook delivery',
    locked_at = NULL, locked_by = NULL, updated_at = now()
FROM webhook_events AS event
WHERE event.id = delivery.event_id AND event.team_id = $1
  AND delivery.status IN ('pending', 'retrying')`, id); err != nil {
		return Team{}, fmt.Errorf("cancel team webhook deliveries: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Team{}, fmt.Errorf("commit disable team transaction: %w", err)
	}
	return teamFromSQLC(row), nil
}

func (r *Repository) GetMember(ctx context.Context, teamID, userID uuid.UUID) (Member, error) {
	row, err := r.queries.GetTeamMember(ctx, dbsqlc.GetTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil {
		return Member{}, fmt.Errorf("get team member: %w", err)
	}
	return memberFromSQLC(row), nil
}

func (r *Repository) GetTenantMembership(ctx context.Context, teamID, userID uuid.UUID) (tenant.Membership, error) {
	member, err := r.GetMember(ctx, teamID, userID)
	if err != nil {
		return tenant.Membership{}, err
	}
	team, err := r.Get(ctx, teamID)
	if err != nil {
		return tenant.Membership{}, err
	}
	return tenant.Membership{TeamID: teamID, UserID: userID, Role: member.Role, Status: member.Status, TeamStatus: team.Status}, nil
}

func (r *Repository) ListMembers(ctx context.Context, teamID uuid.UUID) ([]Member, error) {
	rows, err := r.queries.ListTeamMembers(ctx, dbsqlc.ListTeamMembersParams{TeamID: teamID})
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, memberFromSQLC(row))
	}
	return members, nil
}

func (r *Repository) CreateInvitation(ctx context.Context, teamID uuid.UUID, email, role, tokenHash string, invitedBy uuid.UUID, expiresAt time.Time) (Invitation, error) {
	row, err := r.queries.CreateTeamInvitation(ctx, dbsqlc.CreateTeamInvitationParams{
		TeamID: teamID, Email: email, Role: role, TokenHash: tokenHash,
		InvitedBy: &invitedBy, ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("create team invitation: %w", err)
	}
	return invitationFromSQLC(row), nil
}

func (r *Repository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (Invitation, error) {
	row, err := r.queries.GetTeamInvitationByTokenHash(ctx, dbsqlc.GetTeamInvitationByTokenHashParams{TokenHash: tokenHash})
	if err != nil {
		return Invitation{}, fmt.Errorf("get team invitation by token hash: %w", err)
	}
	return invitationFromSQLC(row), nil
}

func (r *Repository) AcceptInvitation(ctx context.Context, tokenHash string) (Invitation, error) {
	row, err := r.queries.AcceptTeamInvitation(ctx, dbsqlc.AcceptTeamInvitationParams{TokenHash: tokenHash})
	if err != nil {
		return Invitation{}, fmt.Errorf("accept team invitation: %w", err)
	}
	return invitationFromSQLC(row), nil
}

func (r *Repository) AcceptInvitationAndCreateMember(ctx context.Context, tokenHash string, teamID, userID uuid.UUID, role, status string) (Invitation, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Invitation{}, fmt.Errorf("begin invitation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.AcceptTeamInvitation(ctx, dbsqlc.AcceptTeamInvitationParams{TokenHash: tokenHash})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrInvitationNotAccepted
		}
		return Invitation{}, fmt.Errorf("accept team invitation: %w", err)
	}
	if _, err := q.CreateTeamMember(ctx, dbsqlc.CreateTeamMemberParams{TeamID: teamID, UserID: userID, Role: role, Status: status}); err != nil {
		if isUniqueViolation(err) {
			return Invitation{}, ErrTeamMemberAlreadyExists
		}
		return Invitation{}, fmt.Errorf("create team member: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("commit invitation acceptance: %w", err)
	}
	return invitationFromSQLC(row), nil
}

func (r *Repository) DeclineInvitation(ctx context.Context, tokenHash string) (Invitation, error) {
	row, err := r.queries.DeclineTeamInvitation(ctx, dbsqlc.DeclineTeamInvitationParams{TokenHash: tokenHash})
	if err != nil {
		return Invitation{}, fmt.Errorf("decline team invitation: %w", err)
	}
	return invitationFromSQLC(row), nil
}

func (r *Repository) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role string) (Member, error) {
	row, err := r.queries.UpdateTeamMemberRole(ctx, dbsqlc.UpdateTeamMemberRoleParams{TeamID: teamID, UserID: userID, Role: role})
	if err != nil {
		return Member{}, fmt.Errorf("update team member role: %w", err)
	}
	return memberFromSQLC(row), nil
}

func (r *Repository) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	if err := r.queries.RemoveTeamMember(ctx, dbsqlc.RemoveTeamMemberParams{TeamID: teamID, UserID: userID}); err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func teamFromSQLC(row dbsqlc.Team) Team {
	return Team{
		ID: row.ID.String(), Name: row.Name, MarketCode: row.MarketCode,
		Phone: row.Phone, Address: row.Address, Website: row.Website,
		Status: row.Status, CreatedBy: stringPointer(row.CreatedBy),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func stringPointer(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func invitationFromSQLC(row dbsqlc.TeamInvitation) Invitation {
	var invitedBy *string
	if row.InvitedBy != nil {
		value := row.InvitedBy.String()
		invitedBy = &value
	}
	var acceptedAt, declinedAt *time.Time
	if row.AcceptedAt.Valid {
		acceptedAt = &row.AcceptedAt.Time
	}
	if row.DeclinedAt.Valid {
		declinedAt = &row.DeclinedAt.Time
	}
	return Invitation{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Email: row.Email,
		Role: row.Role, Status: row.Status, InvitedBy: invitedBy, ExpiresAt: row.ExpiresAt.Time,
		AcceptedAt: acceptedAt, DeclinedAt: declinedAt, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func memberFromSQLC(row dbsqlc.TeamMember) Member {
	return Member{TeamID: row.TeamID.String(), UserID: row.UserID.String(), Role: row.Role, Status: row.Status, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}
