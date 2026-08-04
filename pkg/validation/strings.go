package validation

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrRequired   = errors.New("value is required")
	ErrTooShort   = errors.New("value is too short")
	ErrTooLong    = errors.New("value is too long")
	ErrNotAllowed = errors.New("value is not allowed")
)

// FieldError identifies the input field that failed validation.
type FieldError struct {
	Field string
	Err   error
}

func (e *FieldError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if strings.TrimSpace(e.Field) == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Field, e.Err)
}

func (e *FieldError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Required trims value and rejects an empty result.
func Required(value string) (string, error) {
	return RequiredField("value", value)
}

func RequiredField(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fieldError(field, ErrRequired)
	}
	return value, nil
}

// Optional trims value and returns nil for an empty result.
func Optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// Length validates a string's Unicode code-point length without modifying it.
// A bound of zero disables that bound.
func Length(value string, minimum, maximum int) error {
	return LengthField("value", value, minimum, maximum)
}

func LengthField(field, value string, minimum, maximum int) error {
	length := utf8.RuneCountInString(value)
	if minimum > 0 && length < minimum {
		return fieldError(field, fmt.Errorf("%w: minimum is %d", ErrTooShort, minimum))
	}
	if maximum > 0 && length > maximum {
		return fieldError(field, fmt.Errorf("%w: maximum is %d", ErrTooLong, maximum))
	}
	return nil
}

// OneOf trims value and returns the canonical allowed value on an exact match.
func OneOf(value string, allowed ...string) (string, error) {
	return OneOfField("value", value, allowed...)
}

func OneOfField(field, value string, allowed ...string) (string, error) {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return candidate, nil
		}
	}
	return "", fieldError(field, ErrNotAllowed)
}

// OneOfFold performs a case-insensitive match and returns the canonical allowed
// value.
func OneOfFold(value string, allowed ...string) (string, error) {
	return OneOfFoldField("value", value, allowed...)
}

func OneOfFoldField(field, value string, allowed ...string) (string, error) {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return candidate, nil
		}
	}
	return "", fieldError(field, ErrNotAllowed)
}

// NormalizeSpace trims surrounding whitespace and collapses internal runs of
// whitespace to a single ASCII space.
func NormalizeSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func fieldError(field string, err error) error {
	return &FieldError{Field: strings.TrimSpace(field), Err: err}
}
