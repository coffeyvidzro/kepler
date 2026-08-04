package authnz

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type cipherKey struct {
	id   string
	aead cipher.AEAD
}

// SecretCipher encrypts with the primary key and decrypts with every key in
// the configured keyring, which allows secrets to be rotated without downtime.
type SecretCipher struct {
	primary cipherKey
	keys    map[string]cipherKey
	ordered []cipherKey
}

func NewSecretCipherKeyring(specifications []string) (*SecretCipher, error) {
	entries := normalizeKeySpecifications(specifications)
	if len(entries) == 0 {
		return nil, ErrEncryptionKeyRequired
	}

	secretCipher := &SecretCipher{keys: make(map[string]cipherKey, len(entries))}
	for _, entry := range entries {
		key, err := parseCipherKey(entry)
		if err != nil {
			return nil, err
		}
		if _, exists := secretCipher.keys[key.id]; exists {
			return nil, fmt.Errorf("%w: duplicate key id %q", ErrInvalidEncryptionKeySpec, key.id)
		}
		secretCipher.keys[key.id] = key
		secretCipher.ordered = append(secretCipher.ordered, key)
	}
	secretCipher.primary = secretCipher.ordered[0]
	return secretCipher, nil
}

func normalizeKeySpecifications(specifications []string) []string {
	entries := make([]string, 0, len(specifications))
	for _, specification := range specifications {
		if specification = strings.TrimSpace(specification); specification != "" {
			entries = append(entries, specification)
		}
	}
	return entries
}

func parseCipherKey(specification string) (cipherKey, error) {
	parts := strings.SplitN(strings.TrimSpace(specification), ":", 2)
	if len(parts) != 2 || !keyIDPattern.MatchString(parts[0]) {
		return cipherKey{}, fmt.Errorf(
			"%w: encryption keys must use key-id:base64-key format",
			ErrInvalidEncryptionKeySpec,
		)
	}
	rawKey, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(rawKey) != 32 {
		return cipherKey{}, fmt.Errorf(
			"%w: encryption key %q must be base64-encoded 32 bytes",
			ErrInvalidEncryptionKey,
			parts[0],
		)
	}
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return cipherKey{}, fmt.Errorf("create encryption key %q: %w", parts[0], err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return cipherKey{}, fmt.Errorf("create encryption key %q GCM: %w", parts[0], err)
	}
	return cipherKey{id: parts[0], aead: aead}, nil
}
