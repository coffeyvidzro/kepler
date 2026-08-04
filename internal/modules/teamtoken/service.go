package teamtoken

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	notifications "github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	defaultTokenTTL = 90 * 24 * time.Hour
	maxTokenTTL     = 365 * 24 * time.Hour
	maxNameLength   = 120
)

var allowedPermissions = map[tenant.Permission]struct{}{
	tenant.PermissionTeamRead:            {},
	tenant.PermissionTeamUpdate:          {},
	tenant.PermissionTeamMembersRead:     {},
	tenant.PermissionTeamMemberInvite:    {},
	tenant.PermissionSenderIDsRead:       {},
	tenant.PermissionSenderIDsCreate:     {},
	tenant.PermissionSenderIDsDelete:     {},
	tenant.PermissionSenderDomainsRead:   {},
	tenant.PermissionSenderDomainsCreate: {},
	tenant.PermissionSenderDomainsDelete: {},
	tenant.PermissionSMSRead:             {},
	tenant.PermissionSMSSend:             {},
	tenant.PermissionEmailRead:           {},
	tenant.PermissionEmailSend:           {},
	tenant.PermissionVerifyRead:          {},
	tenant.PermissionVerifySend:          {},
	tenant.PermissionVerifyCheck:         {},
	tenant.PermissionVerifyManage:        {},
	tenant.PermissionWalletRead:          {},
	tenant.PermissionWalletLedgerRead:    {},
}

type Service struct {
	repository *Repository
	notifier   AdministrativeNotifier
}

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

type AdministrativeNotifier interface {
	SendTeamTokenCreated(context.Context, notifications.SendTeamTokenChangedInput) error
	SendTeamTokenRevoked(context.Context, notifications.SendTeamTokenChangedInput) error
}

func (s *Service) WithNotifier(notifier AdministrativeNotifier) *Service {
	s.notifier = notifier
	return s
}

func (s *Service) List(ctx context.Context) ([]Token, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionTeamTokensRead)
	if err != nil {
		return nil, err
	}
	tokens, err := s.repository.List(ctx, tenantContext.Scope.TeamID)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list team tokens", err)
	}
	return tokens, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (CreatedToken, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionTeamTokensCreate)
	if err != nil {
		return CreatedToken{}, err
	}
	if err := requireOwner(tenantContext); err != nil {
		return CreatedToken{}, err
	}
	name, permissions, expiresAt, err := validateMutation(req.Name, req.Permissions, req.ExpiresAt)
	if err != nil {
		return CreatedToken{}, err
	}
	secret, err := newTeamTokenSecret()
	if err != nil {
		return CreatedToken{}, apperrors.NewInternal("Unable to generate team token", err)
	}
	token, err := s.repository.Create(
		ctx,
		tenantContext.Scope.TeamID,
		name,
		authnz.HashSessionToken(secret),
		tokenDisplayPrefix(secret),
		permissions,
		tenantContext.Actor.UserID,
		expiresAt,
	)
	if err != nil {
		return CreatedToken{}, apperrors.NewInternal("Unable to create team token", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_token.created", ResourceType: "team_token", ResourceID: token.ID, Metadata: map[string]any{"token_prefix": token.TokenPrefix}})
	s.notify(ctx, tenantContext, token, "created")
	return CreatedToken{Token: token, Secret: secret}, nil
}

func (s *Service) Update(ctx context.Context, tokenID string, req UpdateRequest) (Token, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionTeamTokensUpdate)
	if err != nil {
		return Token{}, err
	}
	if err := requireOwner(tenantContext); err != nil {
		return Token{}, err
	}
	parsedTokenID, err := validateTokenID(tokenID)
	if err != nil {
		return Token{}, err
	}
	name, permissions, expiresAt, err := validateMutation(req.Name, req.Permissions, req.ExpiresAt)
	if err != nil {
		return Token{}, err
	}
	token, err := s.repository.Update(
		ctx,
		parsedTokenID,
		tenantContext.Scope.TeamID,
		name,
		permissions,
		expiresAt,
	)
	if err != nil {
		return Token{}, apperrors.NewNotFound("Team token not found")
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_token.updated", ResourceType: "team_token", ResourceID: token.ID, Metadata: map[string]any{"token_prefix": token.TokenPrefix}})
	return token, nil
}

func (s *Service) Revoke(ctx context.Context, tokenID string) (Token, error) {
	tenantContext, err := requireTenantPermission(ctx, tenant.PermissionTeamTokensRevoke)
	if err != nil {
		return Token{}, err
	}
	if err := requireOwner(tenantContext); err != nil {
		return Token{}, err
	}
	parsedTokenID, err := validateTokenID(tokenID)
	if err != nil {
		return Token{}, err
	}
	token, err := s.repository.Revoke(ctx, parsedTokenID, tenantContext.Scope.TeamID)
	if err != nil {
		return Token{}, apperrors.NewNotFound("Team token not found")
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "team_token.revoked", ResourceType: "team_token", ResourceID: token.ID, Metadata: map[string]any{"token_prefix": token.TokenPrefix}})
	s.notify(ctx, tenantContext, token, "revoked")
	return token, nil
}

func (s *Service) notify(ctx context.Context, access tenant.AccessContext, token Token, event string) {
	if s.notifier == nil {
		return
	}
	principal, ok := authnz.PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(principal.Email) == "" {
		return
	}
	input := notifications.SendTeamTokenChangedInput{ToEmail: principal.Email, Name: principal.Name, TeamID: access.Scope.TeamID.String(), TokenName: token.Name, TokenPrefix: token.TokenPrefix}
	var err error
	if event == "created" {
		err = s.notifier.SendTeamTokenCreated(ctx, input)
	} else {
		err = s.notifier.SendTeamTokenRevoked(ctx, input)
	}
	if err != nil {
		slog.Warn("failed to send team token notification", "error", err, "event", event, "team_id", access.Scope.TeamID, "token_id", token.ID)
	}
}

func requireTenantPermission(
	ctx context.Context,
	permission tenant.Permission,
) (tenant.AccessContext, error) {
	tenantContext, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return tenantContext, nil
}

func requireOwner(tenantContext tenant.AccessContext) error {
	if tenantContext.Scope.Role != tenant.RoleOwner {
		return apperrors.NewForbidden("Team owner role is required")
	}
	return nil
}

func newTeamTokenSecret() (string, error) {
	token, err := authnz.NewSessionToken()
	if err != nil {
		return "", err
	}
	return TokenPrefix + token, nil
}

func tokenDisplayPrefix(secret string) string {
	if len(secret) <= 18 {
		return secret
	}
	return secret[:18]
}
