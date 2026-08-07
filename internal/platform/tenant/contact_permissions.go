package tenant

const (
	PermissionContactsRead          Permission = "contacts:read"
	PermissionContactsWrite         Permission = "contacts:write"
	PermissionContactPropertiesRead Permission = "contact_properties:read"
	PermissionContactPropertiesWrite Permission = "contact_properties:write"
)

func init() {
	for _, role := range []string{RoleOwner, RoleAdmin} {
		permissionsByRole[role][PermissionContactsRead] = struct{}{}
		permissionsByRole[role][PermissionContactsWrite] = struct{}{}
		permissionsByRole[role][PermissionContactPropertiesRead] = struct{}{}
		permissionsByRole[role][PermissionContactPropertiesWrite] = struct{}{}
	}
	permissionsByRole[RoleMember][PermissionContactsRead] = struct{}{}
	permissionsByRole[RoleMember][PermissionContactPropertiesRead] = struct{}{}
}
