package team

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct {
	repository *Repository
	notifier   InvitationNotifier
	recipients MemberRecipientStore
}

type InvitationNotifier interface {
	SendTeamInvitation(ctx context.Context, input notifications.SendTeamInvitationInput) error
	SendTeamMemberRemoved(ctx context.Context, input notifications.SendTeamMemberChangedInput) error
	SendTeamMemberRoleChanged(ctx context.Context, input notifications.SendTeamMemberChangedInput) error
}

type MemberRecipientStore interface {
	GetNotificationRecipient(context.Context, uuid.UUID) (notifications.Recipient, error)
}

func NewService(repository *Repository, notifiers ...InvitationNotifier) *Service {
	service := &Service{repository: repository}
	if len(notifiers) > 0 {
		service.notifier = notifiers[0]
	}
	return service
}

func (s *Service) WithRecipientStore(recipients MemberRecipientStore) *Service {
	s.recipients = recipients
	return s
}

const teamInvitationTTL = 7 * 24 * time.Hour

func (s *Service) List(ctx context.Context) ([]Team, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return nil, apperrors.NewUnauthorized("Authentication is required")
	}
	teams, err := s.repository.ListForUser(ctx, principal.UserID)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list teams", err)
	}
	return teams, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Team, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return Team{}, apperrors.NewUnauthorized("Authentication is required")
	}
	name, err := validateTeamName(req.Name)
	if err != nil {
		return Team{}, err
	}
	marketCode, err := validateMarketCode(req.MarketCode)
	if err != nil {
		return Team{}, err
	}
	phone, err := validateRequiredTeamField(req.Phone, "Phone")
	if err != nil {
		return Team{}, err
	}
	address, err := validateRequiredTeamField(req.Address, "Address")
	if err != nil {
		return Team{}, err
	}
	team, err := s.repository.CreateWithOwner(
		ctx, name, marketCode, phone, address, normalizeOptionalTeamField(req.Website), principal.UserID,
	)
	if err != nil {
		return Team{}, apperrors.NewInternal("Unable to create team", err)
	}
	createdTeamID, err := uuid.Parse(team.ID)
	if err != nil {
		return Team{}, apperrors.NewInternal("Unable to parse created team id", err)
	}
	audit.Record(ctx, tenant.AccessContext{
		Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: principal.UserID, SessionID: principal.SessionID},
		Scope: tenant.Scope{TeamID: createdTeamID, Role: tenant.RoleOwner, Status: tenant.StatusActive},
	}, audit.Event{Action: "team.created", ResourceType: "team", ResourceID: team.ID})
	return team, nil
}

func (s *Service) Get(ctx context.Context, teamID string) (Team, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamRead)
	if err != nil {
		return Team{}, err
	}
	team, err := s.repository.Get(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return Team{}, apperrors.NewNotFound("Team not found")
	}
	return team, nil
}

func (s *Service) Update(ctx context.Context, teamID string, req UpdateRequest) (Team, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamUpdate)
	if err != nil {
		return Team{}, err
	}
	name, err := validateTeamName(req.Name)
	if err != nil {
		return Team{}, err
	}
	team, err := s.repository.Update(ctx, tenantContext.Scope.TeamID, name)
	if err != nil {
		return Team{}, apperrors.NewInternal("Unable to update team", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team.updated", ResourceType: "team", ResourceID: team.ID})
	return team, nil
}

func (s *Service) Delete(ctx context.Context, teamID string) (Team, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamDelete)
	if err != nil {
		return Team{}, err
	}
	team, err := s.repository.Disable(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return Team{}, apperrors.NewInternal("Unable to disable team", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team.disabled", ResourceType: "team", ResourceID: team.ID})
	return team, nil
}

func (s *Service) ListMembers(ctx context.Context, teamID string) ([]Member, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamMembersRead)
	if err != nil {
		return nil, err
	}
	members, err := s.repository.ListMembers(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list team members", err)
	}
	return members, nil
}

func (s *Service) Leave(ctx context.Context, teamID string) error {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamMemberLeave)
	if err != nil {
		return err
	}
	if tenantContext.Scope.Role == RoleOwner {
		return apperrors.NewBadRequest("Team owner cannot leave the team")
	}
	if err := s.repository.RemoveMember(ctx, tenantContext.Scope.TeamID, tenantContext.Actor.UserID); err != nil {
		return apperrors.NewInternal("Unable to leave team", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_member.left", ResourceType: "team_member", ResourceID: tenantContext.Actor.UserID.String()})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, teamID string, userID string) error {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamMemberRemove)
	if err != nil {
		return err
	}
	parsedUserID, err := validateMemberID(userID)
	if err != nil {
		return err
	}
	member, err := s.repository.GetMember(ctx, tenantContext.Scope.TeamID, parsedUserID)
	if err != nil {
		return apperrors.NewNotFound("Team member not found")
	}
	if member.Role == RoleOwner {
		return apperrors.NewBadRequest("Team owner cannot be removed")
	}
	if err := s.repository.RemoveMember(ctx, tenantContext.Scope.TeamID, parsedUserID); err != nil {
		return apperrors.NewInternal("Unable to remove team member", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_member.removed", ResourceType: "team_member", ResourceID: parsedUserID.String()})
	s.notifyMemberChange(ctx, tenantContext.Scope.TeamID, parsedUserID, member.Role, "removed")
	return nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, teamID string, userID string, req UpdateMemberRoleRequest) (Member, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamMemberRole)
	if err != nil {
		return Member{}, err
	}
	parsedUserID, err := validateMemberID(userID)
	if err != nil {
		return Member{}, err
	}
	role, err := validateMemberRole(req.Role)
	if err != nil {
		return Member{}, err
	}
	existing, err := s.repository.GetMember(ctx, tenantContext.Scope.TeamID, parsedUserID)
	if err != nil {
		return Member{}, apperrors.NewNotFound("Team member not found")
	}
	if existing.Role == RoleOwner {
		return Member{}, apperrors.NewBadRequest("Team owner role cannot be changed")
	}
	member, err := s.repository.UpdateMemberRole(ctx, tenantContext.Scope.TeamID, parsedUserID, role)
	if err != nil {
		return Member{}, apperrors.NewInternal("Unable to update team member role", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_member.role_updated", ResourceType: "team_member", ResourceID: parsedUserID.String(), Metadata: map[string]any{"role": role}})
	s.notifyMemberChange(ctx, tenantContext.Scope.TeamID, parsedUserID, role, "role_changed")
	return member, nil
}

func (s *Service) notifyMemberChange(ctx context.Context, teamID, userID uuid.UUID, role, event string) {
	if s.notifier == nil || s.recipients == nil {
		return
	}
	recipient, err := s.recipients.GetNotificationRecipient(ctx, userID)
	if err != nil {
		slog.Warn("failed to resolve team member notification recipient", "error", err, "user_id", userID)
		return
	}
	teamRecord, err := s.repository.Get(ctx, teamID)
	if err != nil {
		slog.Warn("failed to resolve team notification context", "error", err, "team_id", teamID)
		return
	}
	input := notifications.SendTeamMemberChangedInput{ToEmail: recipient.Email, Name: recipient.Name, Team: teamRecord.Name, Role: role}
	if event == "removed" {
		err = s.notifier.SendTeamMemberRemoved(ctx, input)
	} else {
		err = s.notifier.SendTeamMemberRoleChanged(ctx, input)
	}
	if err != nil {
		slog.Warn("failed to send team member notification", "error", err, "event", event, "user_id", userID, "team_id", teamID)
	}
}

func (s *Service) InviteMember(ctx context.Context, teamID string, req InviteMemberRequest) (Invitation, error) {
	tenantContext, err := requireTenantPermission(ctx, teamID, tenant.PermissionTeamMemberInvite)
	if err != nil {
		return Invitation{}, err
	}
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return Invitation{}, apperrors.NewUnauthorized("Authentication is required")
	}
	email, err := normalizeInvitationEmail(req.Email)
	if err != nil {
		return Invitation{}, err
	}
	role, err := validateInvitationRole(req.Role)
	if err != nil {
		return Invitation{}, err
	}
	token, err := authnz.NewSessionToken()
	if err != nil {
		return Invitation{}, apperrors.NewInternal("Unable to generate invitation token", err)
	}
	expiresAt := time.Now().UTC().Add(teamInvitationTTL)
	invitation, err := s.repository.CreateInvitation(ctx, tenantContext.Scope.TeamID, email, role, authnz.HashSessionToken(token), tenantContext.Actor.UserID, expiresAt)
	if err != nil {
		return Invitation{}, apperrors.NewInternal("Unable to create team invitation", err)
	}
	invitation.Token = token
	team, err := s.repository.Get(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return Invitation{}, apperrors.NewInternal("Unable to load invited team", err)
	}
	if err := s.sendTeamInvitation(ctx, invitation, team, inviterDisplayName(principal)); err != nil {
		return Invitation{}, err
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_member.invited", ResourceType: "team_invitation", ResourceID: invitation.ID, Metadata: map[string]any{"email": invitation.Email, "role": invitation.Role, "expires_at": invitation.ExpiresAt}})
	return invitation, nil
}

func (s *Service) sendTeamInvitation(ctx context.Context, invitation Invitation, team Team, inviterName string) error {
	if s.notifier == nil {
		return nil
	}
	if err := s.notifier.SendTeamInvitation(ctx, notifications.SendTeamInvitationInput{ToEmail: invitation.Email, TeamName: team.Name, InviterName: inviterName, Role: invitation.Role, Token: invitation.Token}); err != nil {
		return apperrors.NewInternal("Unable to deliver team invitation", err)
	}
	return nil
}

func inviterDisplayName(principal authnz.Principal) string {
	name := strings.TrimSpace(principal.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(principal.Email)
}

func (s *Service) GetInvitation(ctx context.Context, token string) (Invitation, error) {
	principal, invitation, err := s.invitationForPrincipal(ctx, token)
	if err != nil {
		return Invitation{}, err
	}
	if !strings.EqualFold(invitation.Email, principal.Email) {
		return Invitation{}, apperrors.NewForbidden("Invitation does not belong to authenticated user")
	}
	return invitation, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, token string) (Invitation, error) {
	principal, invitation, err := s.invitationForPrincipal(ctx, token)
	if err != nil {
		return Invitation{}, err
	}
	if !strings.EqualFold(invitation.Email, principal.Email) {
		return Invitation{}, apperrors.NewForbidden("Invitation does not belong to authenticated user")
	}
	teamID, err := uuid.Parse(invitation.TeamID)
	if err != nil {
		return Invitation{}, apperrors.NewInternal("Unable to parse invitation team id", err)
	}
	if _, err := s.repository.GetMember(ctx, teamID, principal.UserID); err == nil {
		return Invitation{}, apperrors.NewBadRequest("User is already a team member")
	}
	accepted, err := s.repository.AcceptInvitationAndCreateMember(ctx, authnz.HashSessionToken(normalizeInvitationToken(token)), teamID, principal.UserID, invitation.Role, "active")
	if err != nil {
		switch {
		case errors.Is(err, ErrInvitationNotAccepted):
			return Invitation{}, apperrors.NewBadRequest("Invitation token is invalid or expired")
		case errors.Is(err, ErrTeamMemberAlreadyExists):
			return Invitation{}, apperrors.NewBadRequest("User is already a team member")
		default:
			return Invitation{}, apperrors.NewInternal("Unable to accept invitation", err)
		}
	}
	audit.Record(ctx, tenant.AccessContext{Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: principal.UserID, SessionID: principal.SessionID}, Scope: tenant.Scope{TeamID: teamID, Role: invitation.Role, Status: tenant.StatusActive}}, audit.Event{Action: "team_invitation.accepted", ResourceType: "team_invitation", ResourceID: accepted.ID})
	return accepted, nil
}

func (s *Service) DeclineInvitation(ctx context.Context, token string) (Invitation, error) {
	principal, invitation, err := s.invitationForPrincipal(ctx, token)
	if err != nil {
		return Invitation{}, err
	}
	if !strings.EqualFold(invitation.Email, principal.Email) {
		return Invitation{}, apperrors.NewForbidden("Invitation does not belong to authenticated user")
	}
	declined, err := s.repository.DeclineInvitation(ctx, authnz.HashSessionToken(normalizeInvitationToken(token)))
	if err != nil {
		return Invitation{}, apperrors.NewBadRequest("Invitation token is invalid or expired")
	}
	teamID, _ := uuid.Parse(declined.TeamID)
	audit.Record(ctx, tenant.AccessContext{Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: principal.UserID, SessionID: principal.SessionID}, Scope: tenant.Scope{TeamID: teamID}}, audit.Event{Action: "team_invitation.declined", ResourceType: "team_invitation", ResourceID: declined.ID})
	return declined, nil
}

func (s *Service) invitationForPrincipal(ctx context.Context, token string) (authnz.Principal, Invitation, error) {
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok {
		return authnz.Principal{}, Invitation{}, apperrors.NewUnauthorized("Authentication is required")
	}
	token, err := validateInvitationToken(token)
	if err != nil {
		return authnz.Principal{}, Invitation{}, err
	}
	invitation, err := s.repository.GetInvitationByTokenHash(ctx, authnz.HashSessionToken(token))
	if err != nil {
		return authnz.Principal{}, Invitation{}, apperrors.NewBadRequest("Invitation token is invalid or expired")
	}
	return principal, invitation, nil
}

func requireTenantPermission(ctx context.Context, teamID string, permission tenant.Permission) (tenant.AccessContext, error) {
	tenantContext, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	parsedTeamID, err := validateTeamID(teamID)
	if err != nil {
		return tenant.AccessContext{}, err
	}
	if tenantContext.Scope.TeamID != parsedTeamID {
		return tenant.AccessContext{}, apperrors.NewForbidden("Team context does not match route")
	}
	return tenantContext, nil
}
