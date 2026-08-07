package tenant

const (
	PermissionBroadcastsRead  Permission = "broadcasts:read"
	PermissionBroadcastsWrite Permission = "broadcasts:write"
	PermissionBroadcastsSend  Permission = "broadcasts:send"
)

func init() {
	for _, role := range []string{RoleOwner, RoleAdmin} {
		permissionsByRole[role][PermissionBroadcastsRead] = struct{}{}
		permissionsByRole[role][PermissionBroadcastsWrite] = struct{}{}
		permissionsByRole[role][PermissionBroadcastsSend] = struct{}{}
	}
	permissionsByRole[RoleMember][PermissionBroadcastsRead] = struct{}{}
	permissionsByRole[RoleMember][PermissionBroadcastsWrite] = struct{}{}
}
