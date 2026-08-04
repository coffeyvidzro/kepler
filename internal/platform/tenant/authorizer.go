package tenant

import (
	"context"

	"github.com/google/uuid"
)

type Decision struct {
	Allowed bool
	Reason  string
}

type Authorizer interface {
	Authorize(access AccessContext, permission Permission) Decision
}

type RoleAuthorizer struct{}

func (RoleAuthorizer) Authorize(access AccessContext, permission Permission) Decision {
	if access.Scope.TeamID == uuid.Nil {
		return Decision{Reason: "tenant scope is missing"}
	}
	if access.Scope.Status != StatusActive {
		return Decision{Reason: "active tenant scope is required"}
	}
	if !access.Actor.IsUser() && !access.Actor.IsTeamToken() {
		return Decision{Reason: "authenticated actor is required"}
	}
	if permission == "" {
		return Decision{Allowed: true}
	}
	if access.Actor.IsTeamToken() {
		if HasPermission(access.Scope.Permissions, permission) {
			return Decision{Allowed: true}
		}
		return Decision{Reason: "team token permission is required"}
	}
	if access.Actor.IsUser() && Can(access.Scope.Role, permission) {
		return Decision{Allowed: true}
	}
	return Decision{Reason: "team permission is required"}
}

func DefaultAuthorizer() Authorizer { return RoleAuthorizer{} }

func Authorize(access AccessContext, permission Permission) Decision {
	return DefaultAuthorizer().Authorize(access, permission)
}

func ResolveAccess(ctx context.Context, permission Permission) (AccessContext, Decision) {
	access, ok := AccessFromContext(ctx)
	if !ok {
		return AccessContext{}, Decision{Reason: "team context is required"}
	}
	return access, Authorize(access, permission)
}
