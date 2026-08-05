package authnz

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

var envelopeMagic = []byte("dgb1")

func (secretCipher *SecretCipher) PrimaryKeyID() string {
	if secretCipher == nil {
		return ""
	}
	return secretCipher.primary.id
}

func (secretCipher *SecretCipher) Encrypt(value []byte) ([]byte, error) {
	if secretCipher == nil || secretCipher.primary.aead == nil {
		return nil, ErrEncryptionKeyRequired
	}
	keyID := []byte(secretCipher.primary.id)
	header := make([]byte, len(envelopeMagic)+2+len(keyID))
	copy(header, envelopeMagic)
	binary.BigEndian.PutUint16(header[len(envelopeMagic):], uint16(len(keyID)))
	copy(header[len(envelopeMagic)+2:], keyID)

	nonce := make([]byte, secretCipher.primary.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	output := append(header, nonce...)
	return secretCipher.primary.aead.Seal(output, nonce, value, header), nil
}

func (secretCipher *SecretCipher) Decrypt(value []byte) ([]byte, error) {
	plain, _, err := secretCipher.decrypt(value)
	return plain, err
}

func (secretCipher *SecretCipher) DecryptAndRotate(
	value []byte,
) (plain []byte, replacement []byte, rotated bool, err error) {
	plain, keyID, err := secretCipher.decrypt(value)
	if err != nil {
		return nil, nil, false, err
	}
	if keyID == secretCipher.primary.id {
		return plain, nil, false, nil
	}
	replacement, err = secretCipher.Encrypt(plain)
	if err != nil {
		return nil, nil, false, err
	}
	return plain, replacement, true, nil
}

func (secretCipher *SecretCipher) decrypt(value []byte) ([]byte, string, error) {
	if secretCipher == nil || len(secretCipher.ordered) == 0 {
		return nil, "", ErrEncryptionKeyRequired
	}
	if len(value) >= len(envelopeMagic)+2 && string(value[:len(envelopeMagic)]) == string(envelopeMagic) {
		keyIDLength := int(binary.BigEndian.Uint16(value[len(envelopeMagic):]))
		headerEnd := len(envelopeMagic) + 2 + keyIDLength
		if keyIDLength == 0 || headerEnd > len(value) {
			return nil, "", ErrInvalidEncryptedSecret
		}
		keyID := string(value[len(envelopeMagic)+2 : headerEnd])
		key, ok := secretCipher.keys[keyID]
		if !ok {
			return nil, "", fmt.Errorf("%w: %q", ErrEncryptionKeyNotConfigured, keyID)
		}
		if len(value) < headerEnd+key.aead.NonceSize() {
			return nil, "", ErrInvalidEncryptedSecret
		}
		nonce := value[headerEnd : headerEnd+key.aead.NonceSize()]
		plain, err := key.aead.Open(
			nil,
			nonce,
			value[headerEnd+key.aead.NonceSize():],
			value[:headerEnd],
		)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidEncryptedSecret, err)
		}
		return plain, keyID, nil
	}

	// Legacy envelopes did not carry a key ID. Try the configured keys in order
	// so existing ciphertext remains readable during migration.
	for _, key := range secretCipher.ordered {
		nonceSize := key.aead.NonceSize()
		if len(value) < nonceSize {
			continue
		}
		plain, err := key.aead.Open(nil, value[:nonceSize], value[nonceSize:], nil)
		if err == nil {
			return plain, "", nil
		}
	}
	return nil, "", ErrSecretDecryptionFailed
}
