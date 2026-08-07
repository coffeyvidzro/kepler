package tenant

const (
	PermissionSegmentsRead  Permission = "segments:read"
	PermissionSegmentsWrite Permission = "segments:write"
)

func init() {
	for _, role := range []string{RoleOwner, RoleAdmin} {
		permissionsByRole[role][PermissionSegmentsRead] = struct{}{}
		permissionsByRole[role][PermissionSegmentsWrite] = struct{}{}
	}
	permissionsByRole[RoleMember][PermissionSegmentsRead] = struct{}{}
}
