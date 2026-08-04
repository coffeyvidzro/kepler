package verify

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type codeCipher interface {
	Encrypt([]byte) ([]byte, error)
}

type CodeManager struct {
	secret []byte
	cipher codeCipher
}

type GeneratedCode struct {
	Code       string
	CodeHMAC   []byte
	SealedCode []byte
}

func NewCodeManager(secret []byte, cipher codeCipher) (*CodeManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("verification HMAC secret must be at least 32 bytes")
	}
	if cipher == nil {
		return nil, errors.New("verification code cipher is required")
	}
	return &CodeManager{secret: append([]byte(nil), secret...), cipher: cipher}, nil
}

func (manager *CodeManager) Generate(teamID, verificationID uuid.UUID, sequence, length int32) (GeneratedCode, error) {
	if manager == nil || manager.cipher == nil || len(manager.secret) < 32 {
		return GeneratedCode{}, errors.New("verification code manager is not configured")
	}
	if teamID == uuid.Nil || verificationID == uuid.Nil || sequence <= 0 {
		return GeneratedCode{}, errors.New("verification code context is invalid")
	}
	if length < 4 || length > 10 {
		return GeneratedCode{}, errors.New("verification code length must be between 4 and 10")
	}
	limit := uint64(1)
	for index := int32(0); index < length; index++ {
		limit *= 10
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return GeneratedCode{}, fmt.Errorf("generate verification code: %w", err)
	}
	code := fmt.Sprintf("%0*d", length, binary.BigEndian.Uint64(random[:])%limit)
	sealed, err := manager.cipher.Encrypt([]byte(code))
	if err != nil {
		return GeneratedCode{}, fmt.Errorf("encrypt verification code: %w", err)
	}
	return GeneratedCode{Code: code, CodeHMAC: manager.digest(teamID, verificationID, sequence, code), SealedCode: sealed}, nil
}

func (manager *CodeManager) Matches(teamID, verificationID uuid.UUID, sequence int32, code string, expected []byte) bool {
	if manager == nil || len(manager.secret) < 32 || len(expected) == 0 {
		return false
	}
	actual := manager.digest(teamID, verificationID, sequence, code)
	return hmac.Equal(actual, expected)
}

func (manager *CodeManager) digest(teamID, verificationID uuid.UUID, sequence int32, code string) []byte {
	mac := hmac.New(sha256.New, manager.secret)
	_, _ = mac.Write(teamID[:])
	_, _ = mac.Write(verificationID[:])
	var sequenceBytes [4]byte
	binary.BigEndian.PutUint32(sequenceBytes[:], uint32(sequence))
	_, _ = mac.Write(sequenceBytes[:])
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}
