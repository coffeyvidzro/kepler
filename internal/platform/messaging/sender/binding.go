package sender

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
