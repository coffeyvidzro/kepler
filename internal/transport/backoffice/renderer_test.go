package backoffice

import "testing"

func TestFormatMoneyFromMicros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		micros int64
		want   string
	}{
		{name: "zero", micros: 0, want: "$0.00"},
		{name: "ten dollars", micros: 10_000_000, want: "$10.00"},
		{name: "one dollar and one cent", micros: 1_010_000, want: "$1.01"},
		{name: "preserves mills", micros: 1_015_000, want: "$1.015"},
		{name: "negative", micros: -10_000_000, want: "-$10.00"},
		{name: "sms unit cost", micros: 9_000, want: "$0.009"},
		{name: "negative sms unit cost", micros: -9_000, want: "-$0.009"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatMoney(tt.micros); got != tt.want {
				t.Fatalf("formatMoney(%d) = %q, want %q", tt.micros, got, tt.want)
			}
		})
	}
}
