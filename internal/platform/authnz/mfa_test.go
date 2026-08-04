package authnz

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

func TestTOTPValidation(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	step := now.Unix() / 30
	got, ok := ValidateTOTP(secret, totpCode(secret, step), now)
	if !ok || got != step {
		t.Fatalf("ValidateTOTP() = %d, %t", got, ok)
	}
}

func TestTOTPValidationRejectsMalformedInput(t *testing.T) {
	tests := []struct{ secret, code string }{
		{"not-base32!", ""},
		{"not-base32!", "000000"},
		{"JBSWY3DPEHPK3PXP", "12345"},
		{"JBSWY3DPEHPK3PXP", "12345x"},
	}
	for _, test := range tests {
		if _, ok := ValidateTOTP(test.secret, test.code, time.Now()); ok {
			t.Fatalf("accepted malformed secret %q or code %q", test.secret, test.code)
		}
	}
}

func TestSecretCipher(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := NewSecretCipherKeyring([]string{"test:" + base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := c.Decrypt(sealed)
	if err != nil || string(opened) != "secret" {
		t.Fatalf("Decrypt() = %q, %v", opened, err)
	}
}

func TestRecoveryCodeHashNormalization(t *testing.T) {
	code, err := NewRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if HashRecoveryCode(code) != HashRecoveryCode(code[:8]+code[9:]) {
		t.Fatal("format changed recovery-code hash")
	}
}

func TestSecretCipherKeyRotation(t *testing.T) {
	newKey := bytes.Repeat([]byte{1}, 32)
	oldKey := bytes.Repeat([]byte{2}, 32)
	keyring, err := NewSecretCipherKeyring([]string{"2026-07:" + base64.StdEncoding.EncodeToString(newKey), "2026-01:" + base64.StdEncoding.EncodeToString(oldKey)})
	if err != nil {
		t.Fatal(err)
	}
	oldOnly, err := NewSecretCipherKeyring([]string{"2026-01:" + base64.StdEncoding.EncodeToString(oldKey)})
	if err != nil {
		t.Fatal(err)
	}
	oldCiphertext, err := oldOnly.Encrypt([]byte("rotate-me"))
	if err != nil {
		t.Fatal(err)
	}
	plain, replacement, rotated, err := keyring.DecryptAndRotate(oldCiphertext)
	if err != nil || string(plain) != "rotate-me" || !rotated || len(replacement) == 0 {
		t.Fatalf("rotation = %q, %v, %v", plain, rotated, err)
	}
	plain, replacement, rotated, err = keyring.DecryptAndRotate(replacement)
	if err != nil || string(plain) != "rotate-me" || rotated || replacement != nil {
		t.Fatalf("primary decrypt = %q, %v, %v", plain, rotated, err)
	}
}
func TestSecretCipherRejectsMissingKey(t *testing.T) {
	first, _ := NewSecretCipherKeyring([]string{"first:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))})
	second, _ := NewSecretCipherKeyring([]string{"second:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))})
	ciphertext, _ := first.Encrypt([]byte("secret"))
	if _, err := second.Decrypt(ciphertext); err == nil {
		t.Fatal("decrypted ciphertext without its key")
	}
}

func TestSecretCipherRotatesLegacyCiphertext(t *testing.T) {
	oldKey := bytes.Repeat([]byte{5}, 32)
	block, err := aes.NewCipher(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	legacy := aead.Seal(nonce, nonce, []byte("legacy-secret"), nil)
	keyring, err := NewSecretCipherKeyring([]string{"current:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32)), "previous:" + base64.StdEncoding.EncodeToString(oldKey)})
	if err != nil {
		t.Fatal(err)
	}
	plain, replacement, rotated, err := keyring.DecryptAndRotate(legacy)
	if err != nil || string(plain) != "legacy-secret" || !rotated || len(replacement) == 0 {
		t.Fatalf("legacy rotation = %q, %v, %v", plain, rotated, err)
	}
}
