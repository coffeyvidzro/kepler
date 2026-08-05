// Package pgconv converts between application values and pgx nullable types.
package pgconv

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func UUIDFromString(value string) (pgtype.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.UUID{}, fmt.Errorf("parse uuid: value is required")
	}
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse uuid: %w", err)
	}
	return id, nil
}

func NullableUUID(value *string) (pgtype.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.UUID{}, nil
	}
	return UUIDFromString(*value)
}

func UUIDToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	bytes := id.Bytes
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	)
}

func UUIDToStringPtr(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	value := UUIDToString(id)
	return &value
}

// TextFromString treats an empty string as SQL NULL for compatibility with
// existing repository code.
func TextFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// RequiredText always creates a valid PostgreSQL text value, including for an
// empty string.
func RequiredText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func NullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func TextToString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func TextToStringPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func TimestamptzFromTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func NullableTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return TimestamptzFromTime(*value)
}

func TimestamptzToTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func TimestamptzToTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
