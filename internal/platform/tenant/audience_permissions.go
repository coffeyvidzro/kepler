package tenant

const (
	PermissionTopicsRead         Permission = "topics:read"
	PermissionTopicsWrite        Permission = "topics:write"
	PermissionSuppressionsRead   Permission = "suppressions:read"
	PermissionSuppressionsWrite  Permission = "suppressions:write"
)

func init() {
	for _, role := range []string{RoleOwner, RoleAdmin} {
		permissionsByRole[role][PermissionTopicsRead] = struct{}{}
		permissionsByRole[role][PermissionTopicsWrite] = struct{}{}
		permissionsByRole[role][PermissionSuppressionsRead] = struct{}{}
		permissionsByRole[role][PermissionSuppressionsWrite] = struct{}{}
	}
	permissionsByRole[RoleMember][PermissionTopicsRead] = struct{}{}
	permissionsByRole[RoleMember][PermissionSuppressionsRead] = struct{}{}
}
