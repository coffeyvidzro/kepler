package verify

import (
	"encoding/json"
	"testing"
)

func TestValidateCreateServiceAppliesDefaults(t *testing.T) {
	value, err := validateCreateService(CreateServiceRequest{Key: "login", Name: "Customer login"})
	if err != nil {
		t.Fatalf("validateCreateService() error = %v", err)
	}
	if value.DefaultChannel != ChannelSMS || value.CodeLength != 6 || value.TTLSeconds != 300 || value.MaxAttempts != 5 {
		t.Fatalf("validateCreateService() defaults = %+v", value)
	}
	if value.ResendCooldownSeconds != 30 || value.MaxResends != 3 || !value.Enabled {
		t.Fatalf("validateCreateService() resend defaults = %+v", value)
	}
}

func TestNormalizeRecipient(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		recipient string
		want      string
	}{
		{name: "email", channel: ChannelEmail, recipient: "Customer <USER@Example.COM>", want: "user@example.com"},
		{name: "sms", channel: ChannelSMS, recipient: "+233 (24) 123-4567", want: "+233241234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRecipient(tt.channel, tt.recipient)
			if err != nil {
				t.Fatalf("normalizeRecipient() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeRecipient() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateCreateVerificationRequiresOneServiceReference(t *testing.T) {
	service := VerificationService{DefaultChannel: ChannelSMS}
	_, err := validateCreateVerification(CreateVerificationRequest{
		ServiceID: "id", Service: "login", Recipient: "+233241234567",
	}, service)
	if err == nil {
		t.Fatal("validateCreateVerification() accepted two service references")
	}
}

func TestNormalizeJSONObjectRejectsArray(t *testing.T) {
	if _, err := normalizeJSONObject(json.RawMessage(`[]`)); err == nil {
		t.Fatal("normalizeJSONObject() accepted an array")
	}
}
