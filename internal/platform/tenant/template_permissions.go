package tenant

const (
	PermissionTemplatesRead  Permission = "templates:read"
	PermissionTemplatesWrite Permission = "templates:write"
)

func init() {
	for _, role := range []string{RoleOwner, RoleAdmin} {
		permissionsByRole[role][PermissionTemplatesRead] = struct{}{}
		permissionsByRole[role][PermissionTemplatesWrite] = struct{}{}
	}
	permissionsByRole[RoleMember][PermissionTemplatesRead] = struct{}{}
}
