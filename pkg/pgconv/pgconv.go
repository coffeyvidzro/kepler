package pgconv

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func UUIDFromString(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse uuid: %w", err)
	}

	return id, nil
}

func UUIDToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	b := id.Bytes

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
}

func TextFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}

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

func TimestamptzToTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time
}

func NullableTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}

	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func TimestamptzToTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}

	t := value.Time
	return &t
}
