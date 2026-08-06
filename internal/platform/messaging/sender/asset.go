package sender

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
)

// Asset is the canonical sender identity. Provider, country and region state
// belongs to ProviderBinding rather than the asset itself.
type Asset struct {
	ID                 uuid.UUID
	OwnerType          OwnerType
	TeamID             *uuid.UUID
	Channel            messaging.Channel
	Identity           string
	NormalizedIdentity string
	Purpose            string
	Status             AssetStatus
	HealthStatus       HealthStatus
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// NormalizeIdentity returns the display and comparison forms of a sender
// identity. Email domains are displayed lowercase; SMS casing is preserved.
func NormalizeIdentity(channel messaging.Channel, value string) (display string, normalized string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", errors.New("sender identity is required")
	}
	if !channel.Valid() {
		return "", "", errors.New("sender channel is invalid")
	}
	normalized = strings.ToLower(value)
	if channel == messaging.ChannelEmail {
		display = normalized
	} else {
		display = value
	}
	return display, normalized, nil
}

func (asset Asset) Validate() error {
	if asset.ID == uuid.Nil {
		return errors.New("sender asset ID is required")
	}
	if !asset.OwnerType.Valid() {
		return errors.New("sender asset owner type is invalid")
	}
	switch asset.OwnerType {
	case OwnerPlatform:
		if asset.TeamID != nil {
			return errors.New("platform sender asset cannot have an owning team")
		}
	case OwnerTeam:
		if asset.TeamID == nil || *asset.TeamID == uuid.Nil {
			return errors.New("team sender asset requires an owning team")
		}
	}
	display, normalized, err := NormalizeIdentity(asset.Channel, asset.Identity)
	if err != nil {
		return err
	}
	if asset.Identity != display {
		return errors.New("sender asset identity is not normalized")
	}
	if asset.NormalizedIdentity != normalized {
		return errors.New("sender asset normalized identity does not match identity")
	}
	if !asset.Status.Valid() {
		return errors.New("sender asset status is invalid")
	}
	if !asset.HealthStatus.Valid() {
		return errors.New("sender asset health status is invalid")
	}
	if asset.CreatedAt.IsZero() || asset.UpdatedAt.IsZero() {
		return errors.New("sender asset timestamps are required")
	}
	return nil
}

func (asset Asset) String() string {
	return fmt.Sprintf("%s:%s", asset.Channel, asset.NormalizedIdentity)
}
