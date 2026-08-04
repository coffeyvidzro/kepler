package webhook

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// SigningSecretPrefix identifies a signing secret when it is displayed to a
// customer. The complete value returned by NewSigningSecret, including this
// prefix, is the HMAC key; the encoded portion is not a transport encoding for
// consumers to decode.
const SigningSecretPrefix = "whsec_"

func NewSigningSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate webhook signing secret: %w", err)
	}
	return SigningSecretPrefix + base64.RawURLEncoding.EncodeToString(value), nil
}
