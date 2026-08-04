package webhook

import (
	"strings"
	"testing"
)

func TestSignUsesDisplayedSigningSecretAsKey(t *testing.T) {
	const secret = "whsec_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	const timestamp int64 = 1_700_000_000
	body := []byte(`{"event":"sms.delivered"}`)
	const want = "t=1700000000,v1=a5a2030c220be01d63f6a75eaa7dc3a5c291fec07d07bce95e5d6b29d526879f"

	if got := Sign([]byte(secret), timestamp, body); got != want {
		t.Fatalf("Sign() = %q, want %q", got, want)
	}
	if !VerifySignature([]byte(secret), timestamp, body, want) {
		t.Fatal("VerifySignature() rejected a valid signature")
	}
}

func TestNewSigningSecretUsesPublicPrefix(t *testing.T) {
	secret, err := NewSigningSecret()
	if err != nil {
		t.Fatalf("NewSigningSecret() error = %v", err)
	}
	if !strings.HasPrefix(secret, SigningSecretPrefix) {
		t.Fatalf("NewSigningSecret() = %q, missing prefix", secret)
	}
	if len(secret) <= len(SigningSecretPrefix) {
		t.Fatalf("NewSigningSecret() = %q, missing key material", secret)
	}
}
