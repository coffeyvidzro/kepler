package tenant

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

type ActorType string

const (
	ActorTypeUser      ActorType = "user"
	ActorTypeTeamToken ActorType = "team_token"
)

type Actor struct {
	Type      ActorType
	UserID    uuid.UUID
	SessionID string
	TokenID   uuid.UUID
}

func (a Actor) IsUser() bool { return a.Type == ActorTypeUser && a.UserID != uuid.Nil }

func (a Actor) IsTeamToken() bool {
	return a.Type == ActorTypeTeamToken && a.TokenID != uuid.Nil
}

type Scope struct {
	TeamID      uuid.UUID
	Role        string
	Status      string
	Permissions []Permission
}

type AccessContext struct {
	Actor Actor
	Scope Scope
}

func ContextWithAccess(ctx context.Context, access AccessContext) context.Context {
	return context.WithValue(ctx, contextKey{}, access)
}

func AccessFromContext(ctx context.Context) (AccessContext, bool) {
	access, ok := ctx.Value(contextKey{}).(AccessContext)
	return access, ok
}
