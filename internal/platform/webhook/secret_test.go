package webhook

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewSigningSecret(t *testing.T) {
	first, err := NewSigningSecret()
	if err != nil {
		t.Fatalf("NewSigningSecret() error = %v", err)
	}
	second, err := NewSigningSecret()
	if err != nil {
		t.Fatalf("NewSigningSecret() second error = %v", err)
	}
	if first == second {
		t.Fatal("NewSigningSecret() returned duplicate secrets")
	}
	if !strings.HasPrefix(first, SigningSecretPrefix) {
		t.Fatalf("NewSigningSecret() = %q, missing prefix", first)
	}
	encoded := strings.TrimPrefix(first, SigningSecretPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode signing secret: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded signing secret length = %d, want 32", len(decoded))
	}
}
