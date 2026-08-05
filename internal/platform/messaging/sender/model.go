package sender

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
)

type OwnerType string

const (
	OwnerPlatform OwnerType = "platform"
	OwnerTeam     OwnerType = "team"
)

type AssetStatus string

const (
	AssetStatusPending   AssetStatus = "pending"
	AssetStatusActive    AssetStatus = "active"
	AssetStatusDegraded  AssetStatus = "degraded"
	AssetStatusSuspended AssetStatus = "suspended"
	AssetStatusDisabled  AssetStatus = "disabled"
	AssetStatusFailed    AssetStatus = "failed"
)

type HealthStatus string

const (
	HealthUnknown  HealthStatus = "unknown"
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
)

type BindingStatus string

const (
	BindingStatusPending   BindingStatus = "pending"
	BindingStatusActive    BindingStatus = "active"
	BindingStatusRejected  BindingStatus = "rejected"
	BindingStatusSuspended BindingStatus = "suspended"
	BindingStatusDisabled  BindingStatus = "disabled"
	BindingStatusFailed    BindingStatus = "failed"
	BindingStatusUnknown   BindingStatus = "unknown"
)

type GrantStatus string

const (
	GrantStatusActive  GrantStatus = "active"
	GrantStatusRevoked GrantStatus = "revoked"
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

// ProviderBinding represents one provider-specific approval or verification of
// a sender asset in a country, region and provider account.
type ProviderBinding struct {
	ID                uuid.UUID
	SenderAssetID     uuid.UUID
	Provider          string
	ProviderAccount   string
	Region            string
	CountryCode       string
	ExternalID        string
	Status            BindingStatus
	ProviderStatus    string
	Verified          bool
	HealthStatus      HealthStatus
	VerificationData  json.RawMessage
	SubmittedAt       *time.Time
	VerifiedAt        *time.Time
	RejectedAt        *time.Time
	SuspendedAt       *time.Time
	ExpiresAt         *time.Time
	LastCheckedAt     *time.Time
	NextCheckAt       *time.Time
	Attempts          int32
	LastError         string
	ReconcileLockedAt *time.Time
	ReconcileLockedBy string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

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

func (binding ProviderBinding) Validate() error {
	if binding.ID == uuid.Nil || binding.SenderAssetID == uuid.Nil {
		return errors.New("sender provider binding IDs are required")
	}
	if strings.TrimSpace(binding.Provider) == "" {
		return errors.New("sender provider is required")
	}
	if strings.TrimSpace(binding.ProviderAccount) == "" {
		return errors.New("sender provider account is required")
	}
	if binding.CountryCode != "" && !validCountryCode(binding.CountryCode) {
		return errors.New("sender provider country code is invalid")
	}
	if !binding.Status.Valid() {
		return errors.New("sender provider binding status is invalid")
	}
	if !binding.HealthStatus.Valid() {
		return errors.New("sender provider binding health status is invalid")
	}
	if binding.Attempts < 0 {
		return errors.New("sender provider binding attempts cannot be negative")
	}
	if !validJSONObject(binding.VerificationData) {
		return errors.New("sender provider verification data must be a JSON object")
	}
	if (binding.ReconcileLockedAt == nil) != (strings.TrimSpace(binding.ReconcileLockedBy) == "") {
		return errors.New("sender provider reconciliation lock fields must be set together")
	}
	if binding.CreatedAt.IsZero() || binding.UpdatedAt.IsZero() {
		return errors.New("sender provider binding timestamps are required")
	}
	return nil
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

func (owner OwnerType) Valid() bool {
	switch owner {
	case OwnerPlatform, OwnerTeam:
		return true
	default:
		return false
	}
}

func (status AssetStatus) Valid() bool {
	switch status {
	case AssetStatusPending, AssetStatusActive, AssetStatusDegraded,
		AssetStatusSuspended, AssetStatusDisabled, AssetStatusFailed:
		return true
	default:
		return false
	}
}

func (status HealthStatus) Valid() bool {
	switch status {
	case HealthUnknown, HealthHealthy, HealthDegraded:
		return true
	default:
		return false
	}
}

func (status BindingStatus) Valid() bool {
	switch status {
	case BindingStatusPending, BindingStatusActive, BindingStatusRejected,
		BindingStatusSuspended, BindingStatusDisabled, BindingStatusFailed,
		BindingStatusUnknown:
		return true
	default:
		return false
	}
}

func (status GrantStatus) Valid() bool {
	return status == GrantStatusActive || status == GrantStatusRevoked
}

func validCountryCode(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 2 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func validJSONObject(value json.RawMessage) bool {
	if len(bytes.TrimSpace(value)) == 0 {
		return true
	}
	if !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil
}

func (asset Asset) String() string {
	return fmt.Sprintf("%s:%s", asset.Channel, asset.NormalizedIdentity)
}
