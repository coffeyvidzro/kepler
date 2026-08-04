package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		err   error
	}{
		{name: "valid", value: "  send-123  ", want: "send-123"},
		{name: "missing", value: "  ", err: ErrKeyRequired},
		{name: "too long", value: strings.Repeat("界", MaxKeyRunes+1), err: ErrKeyTooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateKey(test.value)
			if !errors.Is(err, test.err) || got != test.want {
				t.Fatalf("ValidateKey() = %q, %v; want %q, %v", got, err, test.want, test.err)
			}
		})
	}
}
