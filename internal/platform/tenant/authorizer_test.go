package tenant

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestRoleAuthorizer(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	tests := []struct {
		name       string
		access     AccessContext
		permission Permission
		allowed    bool
	}{
		{name: "owner role", access: userAccess(teamID, RoleOwner), permission: PermissionSMSSend, allowed: true},
		{name: "member denied write", access: userAccess(teamID, RoleMember), permission: PermissionSMSSend},
		{name: "member allowed read", access: userAccess(teamID, RoleMember), permission: PermissionSMSRead, allowed: true},
		{name: "token explicit permission", access: tokenAccess(teamID, PermissionSMSSend), permission: PermissionSMSSend, allowed: true},
		{name: "token does not inherit role", access: tokenAccess(teamID, PermissionSMSRead), permission: PermissionSMSSend},
		{name: "missing actor", access: AccessContext{Scope: Scope{TeamID: teamID, Role: RoleOwner, Status: StatusActive}}, permission: PermissionSMSSend},
		{name: "missing scope", access: userAccess(uuid.Nil, RoleOwner), permission: PermissionSMSSend},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision := (RoleAuthorizer{}).Authorize(test.access, test.permission)
			if decision.Allowed != test.allowed {
				t.Fatalf("Authorize() allowed = %t, want %t (reason %q)", decision.Allowed, test.allowed, decision.Reason)
			}
			if !decision.Allowed && decision.Reason == "" {
				t.Fatal("Authorize() denied access without a reason")
			}
		})
	}
}

func TestResolveAccessRequiresContextAndPermission(t *testing.T) {
	t.Parallel()

	if _, decision := ResolveAccess(context.Background(), PermissionSMSRead); decision.Allowed {
		t.Fatal("ResolveAccess() allowed a request without access context")
	}
	access := userAccess(uuid.New(), RoleMember)
	ctx := ContextWithAccess(context.Background(), access)
	got, decision := ResolveAccess(ctx, PermissionSMSRead)
	if !decision.Allowed || !reflect.DeepEqual(got, access) {
		t.Fatalf("ResolveAccess() = %+v, %+v; want supplied access and allowed decision", got, decision)
	}
}

func userAccess(teamID uuid.UUID, role string) AccessContext {
	return AccessContext{
		Actor: Actor{Type: ActorTypeUser, UserID: uuid.New()},
		Scope: Scope{TeamID: teamID, Role: role, Status: StatusActive},
	}
}

func tokenAccess(teamID uuid.UUID, permissions ...Permission) AccessContext {
	return AccessContext{
		Actor: Actor{Type: ActorTypeTeamToken, TokenID: uuid.New()},
		Scope: Scope{TeamID: teamID, Status: StatusActive, Permissions: permissions},
	}
}
