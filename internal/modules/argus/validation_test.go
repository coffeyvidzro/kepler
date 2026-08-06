package argus

import (
	"encoding/json"
	"testing"
)

func TestValidateCreateVerificationDefaults(t *testing.T) {
	t.Parallel()

	validated, err := validateCreateVerification(CreateVerificationRequest{
		Recipient: "+233241234567",
	})
	if err != nil {
		t.Fatalf("validate verification: %v", err)
	}
	if validated.Channel != ChannelSMS {
		t.Fatalf("channel = %q, want %q", validated.Channel, ChannelSMS)
	}
	if validated.RecipientNormalized != "+233241234567" {
		t.Fatalf("normalized recipient = %q", validated.RecipientNormalized)
	}
	if validated.CodeLength != defaultCodeLength {
		t.Fatalf("code length = %d, want %d", validated.CodeLength, defaultCodeLength)
	}
	if validated.TTLSeconds != defaultTTLSeconds {
		t.Fatalf("ttl = %d, want %d", validated.TTLSeconds, defaultTTLSeconds)
	}
	if validated.MaxAttempts != defaultMaxAttempts {
		t.Fatalf("max attempts = %d, want %d", validated.MaxAttempts, defaultMaxAttempts)
	}
	if validated.ResendCooldownSeconds != defaultResendCooldownSeconds {
		t.Fatalf("resend cooldown = %d, want %d", validated.ResendCooldownSeconds, defaultResendCooldownSeconds)
	}
	if validated.MaxResends != defaultMaxResends {
		t.Fatalf("max resends = %d, want %d", validated.MaxResends, defaultMaxResends)
	}
	if string(validated.Metadata) != "{}" {
		t.Fatalf("metadata = %s, want {}", validated.Metadata)
	}
}

func TestValidateCreateVerificationCustomPolicy(t *testing.T) {
	t.Parallel()

	codeLength := int32(8)
	ttlSeconds := int32(600)
	maxAttempts := int32(7)
	maxResends := int32(0)
	validated, err := validateCreateVerification(CreateVerificationRequest{
		Recipient:   "+233 24 123 4567",
		Channel:     " SMS ",
		CodeLength:  &codeLength,
		TTLSeconds:  &ttlSeconds,
		MaxAttempts: &maxAttempts,
		MaxResends:  &maxResends,
		Metadata:    json.RawMessage(`{"purpose":"signup"}`),
	})
	if err != nil {
		t.Fatalf("validate verification: %v", err)
	}
	if validated.RecipientNormalized != "+233241234567" {
		t.Fatalf("normalized recipient = %q", validated.RecipientNormalized)
	}
	if validated.CodeLength != codeLength || validated.TTLSeconds != ttlSeconds {
		t.Fatalf("code policy = (%d, %d), want (%d, %d)", validated.CodeLength, validated.TTLSeconds, codeLength, ttlSeconds)
	}
	if validated.MaxAttempts != maxAttempts {
		t.Fatalf("max attempts = %d, want %d", validated.MaxAttempts, maxAttempts)
	}
	if validated.MaxResends != 0 {
		t.Fatalf("max resends = %d, want 0", validated.MaxResends)
	}
}

func TestValidateCreateVerificationRejectsInvalidPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  CreateVerificationRequest
	}{
		{name: "code length", req: requestWithPolicy(int32Pointer(3), nil, nil, nil)},
		{name: "ttl", req: requestWithPolicy(nil, int32Pointer(29), nil, nil)},
		{name: "attempts", req: requestWithPolicy(nil, nil, int32Pointer(0), nil)},
		{name: "resends", req: requestWithPolicy(nil, nil, nil, int32Pointer(-1))},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := validateCreateVerification(test.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func requestWithPolicy(codeLength, ttlSeconds, maxAttempts, maxResends *int32) CreateVerificationRequest {
	return CreateVerificationRequest{
		Recipient:   "+233241234567",
		CodeLength:  codeLength,
		TTLSeconds:  ttlSeconds,
		MaxAttempts: maxAttempts,
		MaxResends:  maxResends,
	}
}

func int32Pointer(value int32) *int32 { return &value }
