package sender

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
)

// Grant authorizes a team to use an asset. Team-owned assets receive a self
// grant, while platform assets can be granted to many teams.
type Grant struct {
	ID            uuid.UUID
	TeamID        uuid.UUID
	SenderAssetID uuid.UUID
	Channel       messaging.Channel
	Status        GrantStatus
	Default       bool
	Scope         json.RawMessage
	GrantedBy     *uuid.UUID
	GrantedAt     time.Time
	RevokedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (grant Grant) Validate() error {
	if grant.ID == uuid.Nil || grant.TeamID == uuid.Nil || grant.SenderAssetID == uuid.Nil {
		return errors.New("sender asset grant IDs are required")
	}
	if !grant.Channel.Valid() {
		return errors.New("sender asset grant channel is invalid")
	}
	if !grant.Status.Valid() {
		return errors.New("sender asset grant status is invalid")
	}
	if !validJSONObject(grant.Scope) {
		return errors.New("sender asset grant scope must be a JSON object")
	}
	switch grant.Status {
	case GrantStatusActive:
		if grant.RevokedAt != nil {
			return errors.New("active sender asset grant cannot be revoked")
		}
	case GrantStatusRevoked:
		if grant.RevokedAt == nil {
			return errors.New("revoked sender asset grant requires a revocation time")
		}
	}
	if grant.GrantedAt.IsZero() || grant.CreatedAt.IsZero() || grant.UpdatedAt.IsZero() {
		return errors.New("sender asset grant timestamps are required")
	}
	return nil
}
