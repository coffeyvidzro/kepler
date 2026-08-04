package tenant

type Permission string

const (
	PermissionTeamRead            Permission = "team:read"
	PermissionTeamUpdate          Permission = "team:update"
	PermissionTeamDelete          Permission = "team:delete"
	PermissionTeamMembersRead     Permission = "team_members:read"
	PermissionTeamMemberLeave     Permission = "team_members:leave"
	PermissionTeamMemberInvite    Permission = "team_members:invite"
	PermissionTeamMemberRemove    Permission = "team_members:remove"
	PermissionTeamMemberRole      Permission = "team_members:role"
	PermissionTeamTokensRead      Permission = "team_tokens:read"
	PermissionTeamTokensCreate    Permission = "team_tokens:create"
	PermissionTeamTokensUpdate    Permission = "team_tokens:update"
	PermissionTeamTokensRevoke    Permission = "team_tokens:revoke"
	PermissionSenderIDsRead       Permission = "sender_ids:read"
	PermissionSenderIDsCreate     Permission = "sender_ids:create"
	PermissionSenderIDsDelete     Permission = "sender_ids:delete"
	PermissionSenderDomainsRead   Permission = "sender_domains:read"
	PermissionSenderDomainsCreate Permission = "sender_domains:create"
	PermissionSenderDomainsDelete Permission = "sender_domains:delete"
	PermissionSMSRead             Permission = "sms:read"
	PermissionSMSSend             Permission = "sms:send"
	PermissionEmailRead           Permission = "email:read"
	PermissionEmailSend           Permission = "email:send"
	PermissionVerifyRead          Permission = "verify:read"
	PermissionVerifySend          Permission = "verify:send"
	PermissionVerifyCheck         Permission = "verify:check"
	PermissionVerifyManage        Permission = "verify:manage"
	PermissionWebhooksRead        Permission = "webhooks:read"
	PermissionWebhooksWrite       Permission = "webhooks:write"
	PermissionAuditEventsRead     Permission = "audit_events:read"
	PermissionWalletRead          Permission = "wallet:read"
	PermissionWalletLedgerRead    Permission = "wallet_ledger:read"
)

var permissionsByRole = map[string]map[Permission]struct{}{
	RoleOwner: {
		PermissionTeamRead:            {},
		PermissionTeamUpdate:          {},
		PermissionTeamDelete:          {},
		PermissionTeamMembersRead:     {},
		PermissionTeamMemberInvite:    {},
		PermissionTeamMemberRemove:    {},
		PermissionTeamMemberRole:      {},
		PermissionTeamTokensRead:      {},
		PermissionTeamTokensCreate:    {},
		PermissionTeamTokensUpdate:    {},
		PermissionTeamTokensRevoke:    {},
		PermissionSenderIDsRead:       {},
		PermissionSenderIDsCreate:     {},
		PermissionSenderIDsDelete:     {},
		PermissionSenderDomainsRead:   {},
		PermissionSenderDomainsCreate: {},
		PermissionSenderDomainsDelete: {},
		PermissionSMSRead:             {},
		PermissionSMSSend:             {},
		PermissionEmailRead:           {},
		PermissionEmailSend:           {},
		PermissionVerifyRead:          {},
		PermissionVerifySend:          {},
		PermissionVerifyCheck:         {},
		PermissionVerifyManage:        {},
		PermissionWebhooksRead:        {},
		PermissionWebhooksWrite:       {},
		PermissionAuditEventsRead:     {},
		PermissionWalletRead:          {},
		PermissionWalletLedgerRead:    {},
	},
	RoleAdmin: {
		PermissionTeamRead:            {},
		PermissionTeamUpdate:          {},
		PermissionTeamMembersRead:     {},
		PermissionTeamMemberLeave:     {},
		PermissionTeamMemberInvite:    {},
		PermissionTeamMemberRemove:    {},
		PermissionTeamTokensRead:      {},
		PermissionTeamTokensCreate:    {},
		PermissionTeamTokensUpdate:    {},
		PermissionTeamTokensRevoke:    {},
		PermissionSenderIDsRead:       {},
		PermissionSenderIDsCreate:     {},
		PermissionSenderIDsDelete:     {},
		PermissionSenderDomainsRead:   {},
		PermissionSenderDomainsCreate: {},
		PermissionSenderDomainsDelete: {},
		PermissionSMSRead:             {},
		PermissionSMSSend:             {},
		PermissionEmailRead:           {},
		PermissionEmailSend:           {},
		PermissionVerifyRead:          {},
		PermissionVerifySend:          {},
		PermissionVerifyCheck:         {},
		PermissionVerifyManage:        {},
		PermissionWebhooksRead:        {},
		PermissionWebhooksWrite:       {},
		PermissionAuditEventsRead:     {},
		PermissionWalletRead:          {},
		PermissionWalletLedgerRead:    {},
	},
	RoleMember: {
		PermissionTeamRead:          {},
		PermissionTeamMembersRead:   {},
		PermissionTeamMemberLeave:   {},
		PermissionSenderIDsRead:     {},
		PermissionSenderDomainsRead: {},
		PermissionSMSRead:           {},
		PermissionEmailRead:         {},
		PermissionVerifyRead:        {},
		PermissionWebhooksRead:      {},
	},
}

func Can(role string, permission Permission) bool {
	permissions, ok := permissionsByRole[role]
	if !ok {
		return false
	}
	_, ok = permissions[permission]
	return ok
}

func HasPermission(permissions []Permission, permission Permission) bool {
	for _, candidate := range permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
