package authnz

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const DefaultTokenBytes = 32

// NewToken returns a cryptographically secure URL-safe opaque token.
func NewToken() (string, error) {
	return NewTokenWithSize(DefaultTokenBytes)
}

func NewTokenWithSize(size int) (string, error) {
	if size <= 0 {
		return "", ErrInvalidTokenSize
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// HashToken returns a stable, non-reversible representation suitable for
// storing opaque tokens in a database.
func HashToken(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
