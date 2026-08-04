package idempotency

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	Header           = "Idempotency-Key"
	MaxKeyRunes      = 256
)

var (
	ErrKeyRequired = errors.New("idempotency key is required")
	ErrKeyTooLong  = errors.New("idempotency key is too long")
)

func ValidateKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrKeyRequired
	}
	if utf8.RuneCountInString(value) > MaxKeyRunes {
		return "", ErrKeyTooLong
	}
	return value, nil
}

type Record struct {
	Scope               string
	Key                 string
	Method              string
	Path                string
	RequestHash         string
	Status              string
	ResponseStatus      *int32
	ResponseBody        []byte
	ResponseContentType *string
	ResponseHeaders     []byte
	LockedUntil         time.Time
	CompletedAt         *time.Time
	ExpiresAt           time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
