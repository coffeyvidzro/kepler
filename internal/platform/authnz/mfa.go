package authnz

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var envelopeMagic = []byte("dgb1")

type cipherKey struct {
	id   string
	aead cipher.AEAD
}
type SecretCipher struct {
	primary cipherKey
	keys    map[string]cipherKey
	ordered []cipherKey
}

func NewSecretCipherKeyring(specs []string) (*SecretCipher, error) {
	entries := make([]string, 0, len(specs))
	for _, s := range specs {
		if strings.TrimSpace(s) != "" {
			entries = append(entries, strings.TrimSpace(s))
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("at least one encryption key is required")
	}
	c := &SecretCipher{keys: map[string]cipherKey{}}
	for _, entry := range entries {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 || !keyIDPattern.MatchString(parts[0]) {
			return nil, errors.New("encryption keys must use key-id:base64-key format")
		}
		if _, ok := c.keys[parts[0]]; ok {
			return nil, fmt.Errorf("duplicate encryption key id %q", parts[0])
		}
		raw, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil || len(raw) != 32 {
			return nil, fmt.Errorf("encryption key %q must be base64-encoded 32 bytes", parts[0])
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		k := cipherKey{id: parts[0], aead: aead}
		c.keys[k.id] = k
		c.ordered = append(c.ordered, k)
	}
	c.primary = c.ordered[0]
	return c, nil
}
func (c *SecretCipher) PrimaryKeyID() string { return c.primary.id }
func (c *SecretCipher) Encrypt(value []byte) ([]byte, error) {
	id := []byte(c.primary.id)
	header := make([]byte, len(envelopeMagic)+2+len(id))
	copy(header, envelopeMagic)
	binary.BigEndian.PutUint16(header[len(envelopeMagic):], uint16(len(id)))
	copy(header[len(envelopeMagic)+2:], id)
	nonce := make([]byte, c.primary.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append(header, nonce...)
	return c.primary.aead.Seal(out, nonce, value, header), nil
}
func (c *SecretCipher) Decrypt(value []byte) ([]byte, error) {
	plain, _, err := c.decrypt(value)
	return plain, err
}
func (c *SecretCipher) DecryptAndRotate(value []byte) (plain, replacement []byte, rotated bool, err error) {
	plain, id, err := c.decrypt(value)
	if err != nil {
		return nil, nil, false, err
	}
	if id == c.primary.id {
		return plain, nil, false, nil
	}
	replacement, err = c.Encrypt(plain)
	if err != nil {
		return nil, nil, false, err
	}
	return plain, replacement, true, nil
}
func (c *SecretCipher) decrypt(value []byte) ([]byte, string, error) {
	if len(value) >= len(envelopeMagic)+2 && string(value[:len(envelopeMagic)]) == string(envelopeMagic) {
		n := int(binary.BigEndian.Uint16(value[len(envelopeMagic):]))
		end := len(envelopeMagic) + 2 + n
		if n == 0 || end > len(value) {
			return nil, "", errors.New("invalid encrypted secret envelope")
		}
		id := string(value[len(envelopeMagic)+2 : end])
		k, ok := c.keys[id]
		if !ok {
			return nil, "", fmt.Errorf("encryption key %q is not configured", id)
		}
		if len(value) < end+k.aead.NonceSize() {
			return nil, "", errors.New("invalid encrypted secret")
		}
		nonce := value[end : end+k.aead.NonceSize()]
		plain, err := k.aead.Open(nil, nonce, value[end+k.aead.NonceSize():], value[:end])
		return plain, id, err
	}
	for _, k := range c.ordered {
		n := k.aead.NonceSize()
		if len(value) < n {
			continue
		}
		if plain, err := k.aead.Open(nil, value[:n], value[n:], nil); err == nil {
			return plain, "", nil
		}
	}
	return nil, "", errors.New("unable to decrypt secret with configured keys")
}

func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func TOTPURI(issuer, account, secret string) string {
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "period": {"30"}, "digits": {"6"}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}

func ValidateTOTP(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
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
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := (uint32(sum[offset])&127)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func NewRecoveryCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	v := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return v[:8] + "-" + v[8:], nil
}

func HashRecoveryCode(code string) string {
	value := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "-", "")
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
