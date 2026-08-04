package verify

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

type testCipher struct{ plaintext []byte }

func (cipher *testCipher) Encrypt(value []byte) ([]byte, error) {
	cipher.plaintext = append([]byte(nil), value...)
	return append([]byte("sealed:"), value...), nil
}

func TestCodeManagerGenerateAndMatch(t *testing.T) {
	cipher := &testCipher{}
	manager, err := NewCodeManager(bytes.Repeat([]byte{7}, 32), cipher)
	if err != nil {
		t.Fatalf("NewCodeManager() error = %v", err)
	}
	teamID := uuid.New()
	verificationID := uuid.New()
	generated, err := manager.Generate(teamID, verificationID, 1, 6)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(cipher.plaintext) != 6 || len(generated.CodeHMAC) != 32 || len(generated.SealedCode) == 0 {
		t.Fatalf("Generate() returned invalid challenge: %+v", generated)
	}
	code := string(cipher.plaintext)
	if !manager.Matches(teamID, verificationID, 1, code, generated.CodeHMAC) {
		t.Fatal("Matches() rejected generated code")
	}
	if manager.Matches(teamID, verificationID, 2, code, generated.CodeHMAC) {
		t.Fatal("Matches() accepted a code for a different challenge sequence")
	}
}

func TestNewCodeManagerRejectsShortSecret(t *testing.T) {
	if _, err := NewCodeManager([]byte("short"), &testCipher{}); err == nil {
		t.Fatal("NewCodeManager() accepted a short HMAC secret")
	}
}
