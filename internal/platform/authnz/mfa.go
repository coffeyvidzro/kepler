package authnz

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func NewTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

func TOTPURI(issuer, account, secret string) string {
	values := url.Values{
		"secret": {secret},
		"issuer": {issuer},
		"period": {"30"},
		"digits": {"6"},
	}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + values.Encode()
}

func ValidateTOTP(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 || strings.IndexFunc(code, func(value rune) bool {
		return value < '0' || value > '9'
	}) >= 0 {
		return 0, false
	}
	step := now.Unix() / 30
	for offset := int64(-1); offset <= 1; offset++ {
		expected := totpCode(secret, step+offset)
		if expected != "" && hmac.Equal([]byte(expected), []byte(code)) {
			return step + offset, true
		}
	}
	return 0, false
}

func totpCode(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := (uint32(digest[offset])&127)<<24 |
		uint32(digest[offset+1])<<16 |
		uint32(digest[offset+2])<<8 |
		uint32(digest[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func NewRecoveryCode() (string, error) {
	value := make([]byte, 10)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:], nil
}

func HashRecoveryCode(code string) string {
	value := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "-", "")
	digest := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
