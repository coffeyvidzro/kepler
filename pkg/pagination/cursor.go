// Package pagination provides reusable offset and cursor pagination primitives.
package pagination

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const MaxCursorLength = 4096

var (
	ErrInvalidCursor = errors.New("invalid pagination cursor")
	ErrCursorTooLong = errors.New("pagination cursor is too long")
)

// EncodeCursor serializes value into an opaque URL-safe cursor.
func EncodeCursor[T any](value T) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode pagination cursor: %w", err)
	}
	cursor := base64.RawURLEncoding.EncodeToString(payload)
	if len(cursor) > MaxCursorLength {
		return "", ErrCursorTooLong
	}
	return cursor, nil
}

// DecodeCursor decodes a cursor produced by EncodeCursor. Unknown JSON fields
// are rejected so cursor schema changes fail closed.
func DecodeCursor[T any](cursor string) (T, error) {
	var result T
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return result, ErrInvalidCursor
	}
	if len(cursor) > MaxCursorLength {
		return result, ErrCursorTooLong
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return result, fmt.Errorf("%w: invalid base64", ErrInvalidCursor)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("%w: invalid payload", ErrInvalidCursor)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return result, fmt.Errorf("%w: trailing payload", ErrInvalidCursor)
	}
	return result, nil
}

// DecodeOptionalCursor treats a blank cursor as absent rather than invalid.
func DecodeOptionalCursor[T any](cursor string) (value T, present bool, err error) {
	if strings.TrimSpace(cursor) == "" {
		return value, false, nil
	}
	value, err = DecodeCursor[T](cursor)
	return value, true, err
}

// CursorPage is a cursor-paginated response.
type CursorPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

func NewCursorPage[T any](items []T, nextCursor string) CursorPage[T] {
	nextCursor = strings.TrimSpace(nextCursor)
	return CursorPage[T]{Items: items, NextCursor: nextCursor, HasMore: nextCursor != ""}
}
