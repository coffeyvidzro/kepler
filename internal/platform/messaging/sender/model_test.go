package sender

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
)

func TestNormalizeIdentity(t *testing.T) {
	t.Parallel()

	display, normalized, err := NormalizeIdentity(messaging.ChannelEmail, " Example.COM ")
	if err != nil {
		t.Fatalf("NormalizeIdentity(email) error = %v", err)
	}
	if display != "example.com" || normalized != "example.com" {
		t.Fatalf("NormalizeIdentity(email) = %q, %q", display, normalized)
	}

	display, normalized, err = NormalizeIdentity(messaging.ChannelSMS, " Dugble ")
	if err != nil {
		t.Fatalf("NormalizeIdentity(SMS) error = %v", err)
	}
	if display != "Dugble" || normalized != "dugble" {
		t.Fatalf("NormalizeIdentity(SMS) = %q, %q", display, normalized)
	}
}

func TestAssetValidateOwnership(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	teamID := uuid.New()
	asset := Asset{
		ID:                 uuid.New(),
		OwnerType:          OwnerTeam,
		TeamID:             &teamID,
		Channel:            messaging.ChannelSMS,
		Identity:           "Dugble",
		NormalizedIdentity: "dugble",
		Status:             AssetStatusActive,
		HealthStatus:       HealthHealthy,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := asset.Validate(); err != nil {
		t.Fatalf("Asset.Validate() error = %v", err)
	}

	asset.OwnerType = OwnerPlatform
	if err := asset.Validate(); err == nil {
		t.Fatal("Asset.Validate() error = nil for platform asset with team owner")
	}
}

func TestProviderBindingValidate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	binding := ProviderBinding{
		ID:               uuid.New(),
		SenderAssetID:    uuid.New(),
		Provider:         "moolre",
		ProviderAccount:  "default",
		CountryCode:      "GH",
		Status:           BindingStatusActive,
		HealthStatus:     HealthHealthy,
		VerificationData: json.RawMessage(`{"whitelisted":true}`),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("ProviderBinding.Validate() error = %v", err)
	}

	binding.CountryCode = "GHA"
	if err := binding.Validate(); err == nil {
		t.Fatal("ProviderBinding.Validate() error = nil for invalid country")
	}
}
